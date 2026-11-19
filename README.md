# Event Reconciliation Platform

Day 1 publishes four synthetic commerce events, including a duplicate payment and a refund that arrives before the payment. Every record uses its order ID as the partition key. Amounts are integer cents so later reconciliation can use exact arithmetic.

Redpanda holds the replayable event log. The Go emitter checks broker acknowledgments before reporting success. Docker Compose brings up the broker, creates the topic, and runs the emitter.

```sh
docker compose up --build -d
docker compose logs -f emitter
docker compose exec broker rpk topic consume order-events -n 4 --offset start
go test ./...
```

Next milestones add transactional reconciliation in PostgreSQL, replay and crash tests, and a FastAPI evidence endpoint for LLM explanations.
