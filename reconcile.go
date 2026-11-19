package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

//go:embed schema.sql
var schema string

const insertEvent = `
WITH event AS (SELECT * FROM jsonb_to_record(:'event'::jsonb)
    AS data(event_id text, order_id text, kind text, amount_cents bigint))
INSERT INTO reconciliation_events SELECT * FROM event
ON CONFLICT (event_id) DO UPDATE SET event_id = excluded.event_id
WHERE (reconciliation_events.order_id, reconciliation_events.kind, reconciliation_events.amount_cents)
    = (excluded.order_id, excluded.kind, excluded.amount_cents)
RETURNING 1;
`

func query(ctx context.Context, sql string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "psql", append([]string{"-X", "-qAt", "-w", "-v", "ON_ERROR_STOP=1"}, args...)...)
	cmd.Stdin = strings.NewReader(sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("psql: %w: %s", err, stderr.String())
	}
	return out, nil
}

func saveEvent(ctx context.Context, event Event) error {
	if event.ID == "" || event.Order == "" {
		return errors.New("event_id and order_id are required")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	out, err := query(ctx, insertEvent, "-v", "event="+string(payload))
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "1" {
		return fmt.Errorf("event ID %q already has a different payload", event.ID)
	}
	return nil
}

func brokerRequest(ctx context.Context, client *http.Client, method, url, payload string, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/vnd.kafka.v2+json")
	if method == http.MethodGet {
		req.Header.Set("Accept", "application/vnd.kafka.json.v2+json")
	} else {
		req.Header.Set("Accept", "application/vnd.kafka.v2+json")
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s returned %s", method, url, res.Status)
	}
	if result != nil {
		return json.NewDecoder(res.Body).Decode(result)
	}
	return nil
}

func reconcile(ctx context.Context, endpoint string) error {
	if _, err := query(ctx, schema); err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	group := fmt.Sprintf("%s/consumers/reconciliation-%d", endpoint, time.Now().UnixNano())
	var consumer struct {
		Instance string `json:"instance_id"`
	}
	if err := brokerRequest(ctx, client, "POST", group,
		`{"name":"worker","format":"json","auto.offset.reset":"earliest","auto.commit.enable":"false"}`, &consumer); err != nil {
		return err
	}
	instance := group + "/instances/" + consumer.Instance
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = brokerRequest(cleanup, client, "DELETE", instance, "", nil)
	}()
	if err := brokerRequest(ctx, client, "POST", instance+"/subscription", `{"topics":["order-events"]}`, nil); err != nil {
		return err
	}
	for ctx.Err() == nil {
		var records []struct {
			Value Event `json:"value"`
		}
		if err := brokerRequest(ctx, client, "GET", instance+"/records?timeout=1000", "", &records); err != nil {
			return err
		}
		for _, record := range records {
			if err := saveEvent(ctx, record.Value); err != nil {
				return err
			}
		}
		if len(records) > 0 {
			fmt.Fprintf(os.Stdout, "Reconciled %d broker records\n", len(records))
		}
	}
	return ctx.Err()
}
