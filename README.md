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

Run `sh scripts/verify-recovery.sh` to verify duplicate handling and recovery against Redpanda and PostgreSQL. The check starts an isolated Compose project, verifies the ledger after ingest, kills the reconciler, publishes the fixture again while it is down, then checks that the restarted worker consumes all eight records and leaves exactly three ledger entries with the same totals.

After the reconciler initializes the ledger, run `docker compose up --build -d evidence` to serve read-only evidence on localhost. Request `GET /tools/orders/order-1` to retrieve the status and exact cent amounts for the synthetic fixture. Amounts are decimal strings to preserve integer precision.

Set `LLM_CHAT_URL` to a reachable OpenAI-compatible chat completions endpoint and set `LLM_MODEL` (plus `LLM_API_KEY` if required). Call `POST /explain/orders/order-1` to get a model-selected focus, a server-rendered explanation, and the evidence row and link. The server validates the focus and derives all published claims and amounts from the ledger row.
