#!/bin/sh
set -eu

# Use a fresh project so the fixture never touches an existing ledger.
COMPOSE_PROJECT_NAME="recovery-$(date +%s)-$$"
export COMPOSE_PROJECT_NAME
trap 'docker compose down -v' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

wait_for_batch() {
    attempts=0
    until docker compose logs --no-color reconciler | awk '/Reconciled [0-9]+ broker records/ {total += $(NF - 2)} END {exit (total < 4)}'; do
        attempts=$((attempts + 1))
        if [ "$attempts" -ge 60 ]; then
            docker compose logs reconciler
            return 1
        fi
        sleep 1
    done
}

assert_ledger() {
    actual=$(docker compose exec -T postgres psql -XAt -v ON_ERROR_STOP=1 -U postgres -c "
SELECT count(*) FROM reconciliation_events;
SELECT order_id, expected_cents, paid_cents, refunded_cents, net_cents, status
FROM order_reconciliation ORDER BY order_id;")
    expected='3
order-1|12000|12000|2500|9500|reconciled'
    if [ "$actual" != "$expected" ]; then
        printf 'Unexpected ledger:\n%s\n' "$actual" >&2
        return 1
    fi
}

docker compose up --build -d reconciler
docker compose run --build --rm --no-deps emitter
wait_for_batch
assert_ledger
docker compose kill -s SIGKILL reconciler
docker compose run --rm --no-deps emitter

# Recreate only the worker. The broker log and PostgreSQL ledger survive.
docker compose up -d --no-deps --force-recreate reconciler
attempts=0
while :; do
    processed=$(docker compose logs --no-color reconciler |
        awk '/Reconciled [0-9]+ broker records/ {total += $(NF - 2)} END {print total+0}')
    [ "$processed" -ge 8 ] && break
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 60 ]; then
        docker compose logs reconciler
        exit 1
    fi
    sleep 1
done
assert_ledger
printf 'Verified duplicate handling and recovery after replaying %s records.\n' "$processed"
