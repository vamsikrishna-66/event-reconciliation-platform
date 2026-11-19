package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Event struct {
	ID    string `json:"event_id"`
	Order string `json:"order_id"`
	Kind  string `json:"kind"`
	Cents int    `json:"amount_cents"`
}

func fixture() []Event {
	order := Event{"order-1-created", "order-1", "order_created", 12000}
	payment := Event{"order-1-paid", "order-1", "payment_captured", 12000}
	refund := Event{"order-1-refunded", "order-1", "refund_issued", 2500}
	return []Event{order, refund, payment, payment}
}

func publish(client *http.Client, endpoint string, events []Event) error {
	type record struct {
		Key   string `json:"key"`
		Value Event  `json:"value"`
	}
	payload := struct {
		Records []record `json:"records"`
	}{}
	for _, event := range events {
		payload.Records = append(payload.Records, record{event.Order, event})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	res, err := client.Post(endpoint+"/topics/order-events", "application/vnd.kafka.json.v2+json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("broker returned %s", res.Status)
	}
	var report struct {
		Offsets []struct {
			ErrorCode *int `json:"error_code"`
		} `json:"offsets"`
	}
	if err := json.NewDecoder(res.Body).Decode(&report); err != nil {
		return err
	}
	if len(report.Offsets) != len(events) {
		return fmt.Errorf("expected %d acknowledgments, got %d", len(events), len(report.Offsets))
	}
	for _, offset := range report.Offsets {
		if offset.ErrorCode != nil && *offset.ErrorCode != 0 {
			return fmt.Errorf("broker rejected record with code %d", *offset.ErrorCode)
		}
	}
	return nil
}

func main() {
	endpoint := os.Getenv("BROKER_HTTP_URL")
	if endpoint == "" {
		endpoint = "http://localhost:8082"
	}
	if len(os.Args) == 2 && os.Args[1] == "reconcile" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := reconcile(ctx, endpoint); err != nil && ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	events := fixture()
	if err := publish(&http.Client{Timeout: 10 * time.Second}, endpoint, events); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Broker acknowledged %d events\n", len(events))
}
