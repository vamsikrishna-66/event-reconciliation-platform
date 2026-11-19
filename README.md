# Event Reconciliation Platform

The Go emitter publishes synthetic commerce events to Redpanda, including a duplicate payment and a refund that arrives before its payment. Records use the order ID as their partition key and amounts use integer cents.

The Go consumer stores events in PostgreSQL under their unique event IDs. An identical event keeps the same ledger entry. A reused ID with a different payload stops the consumer and reports the conflict. The reconciliation view derives the expected, paid, refunded, and net amounts and flags mismatches regardless of arrival order. Each consumer start reads from the earliest retained broker record and reuses the existing ledger.

```sh
docker compose up --build -d
docker compose logs reconciler
docker compose exec postgres psql -U postgres -c 'TABLE order_reconciliation'
go test ./...
```

The fixture produces one reconciled order with 12,000 cents paid, 2,500 cents refunded, and 9,500 cents net.
