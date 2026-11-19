package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublishPreservesDuplicateAndArrivalOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Records []struct {
				Key   string
				Value Event
			}
		}
		if r.Method != "POST" || r.URL.Path != "/topics/order-events" || json.NewDecoder(r.Body).Decode(&payload) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		if len(payload.Records) != 4 || payload.Records[1].Value.Kind != "refund_issued" ||
			payload.Records[2].Value != payload.Records[3].Value || payload.Records[0].Key != "order-1" {
			http.Error(w, "invalid fixture", 400)
			return
		}
		w.Write([]byte(`{"offsets":[{},{},{},{}]}`))
	}))
	defer server.Close()
	if err := publish(server.Client(), server.URL, fixture()); err != nil {
		t.Fatal(err)
	}
}
