#!/usr/bin/env bash
# Run the money-path integration test (P0-E) against EPHEMERAL Postgres+Redis
# containers — never the production database. Safe to run on the droplet:
# uses the already-pulled alpine images, loopback-only high ports, and caps
# Go compiler memory (1GB box, see ENGINEERING_STANDARDS.md / OOM history).
#
# Usage: scripts/integration-test.sh [extra go test args]
set -euo pipefail

REPO=/var/www/myfibase
PG_NAME=myfibase_itest_pg
REDIS_NAME=myfibase_itest_redis
PG_PORT=55432
REDIS_PORT=56379

cleanup() {
    docker rm -f "$PG_NAME" "$REDIS_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup  # remove leftovers from a previous interrupted run

docker run -d --rm --name "$PG_NAME" \
    -e POSTGRES_DB=itest -e POSTGRES_USER=itest -e POSTGRES_PASSWORD=itest \
    -p "127.0.0.1:$PG_PORT:5432" \
    --tmpfs /var/lib/postgresql/data \
    postgres:16-alpine >/dev/null
docker run -d --rm --name "$REDIS_NAME" \
    -p "127.0.0.1:$REDIS_PORT:6379" \
    redis:7-alpine >/dev/null

for i in $(seq 1 30); do
    docker exec "$PG_NAME" pg_isready -U itest -d itest >/dev/null 2>&1 && break
    [ "$i" -eq 30 ] && { echo "postgres never became ready" >&2; exit 1; }
    sleep 1
done

cd "$REPO/api"
TEST_DATABASE_URL="postgres://itest:itest@127.0.0.1:$PG_PORT/itest?sslmode=disable" \
TEST_REDIS_URL="redis://127.0.0.1:$REDIS_PORT/0" \
GOFLAGS=-p=1 GOMEMLIMIT=200MiB \
    nice -n 19 go test -tags integration ./integration/ -count=1 -v "$@"
