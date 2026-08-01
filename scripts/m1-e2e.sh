#!/bin/sh

set -eu

test_project=hubcr-m1-e2e
test_compose=deployments/compose/compose.integration.yaml
test_port=${HUBCR_M1_E2E_POSTGRES_PORT:-55433}
api_port=${HUBCR_M1_E2E_API_PORT:-18080}
web_port=${HUBCR_M1_E2E_WEB_PORT:-3100}
database_url="postgres://hubcr_test:hubcr-test-only@127.0.0.1:${test_port}/hubcr_test?sslmode=disable"
test_password=m1-e2e-password
log_directory=$(mktemp -d)
api_pid=
web_pid=

cleanup() {
    if [ -n "$web_pid" ]; then
        kill "$web_pid" 2>/dev/null || true
        wait "$web_pid" 2>/dev/null || true
    fi
    if [ -n "$api_pid" ]; then
        kill "$api_pid" 2>/dev/null || true
        wait "$api_pid" 2>/dev/null || true
    fi
    HUBCR_TEST_POSTGRES_PORT="$test_port" docker compose --project-name "$test_project" --file "$test_compose" down --volumes --remove-orphans
    rm -rf "$log_directory"
}

wait_for_url() {
    url=$1
    attempts=0
    until curl --fail --silent "$url" >/dev/null; do
        attempts=$((attempts + 1))
        if [ "$attempts" -ge 40 ]; then
            return 1
        fi
        sleep 0.25
    done
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

HUBCR_TEST_POSTGRES_PORT="$test_port" docker compose --project-name "$test_project" --file "$test_compose" up --detach --wait

HUBCR_DATABASE_URL="$database_url" HUBCR_E2E_PASSWORD="$test_password" \
    go -C backend run ./internal/testsupport/m1seed

HUBCR_DATABASE_URL="$database_url" HUBCR_API_ADDRESS="127.0.0.1:${api_port}" \
    go -C backend run ./cmd/api >"$log_directory/api.log" 2>&1 &
api_pid=$!
if ! wait_for_url "http://127.0.0.1:${api_port}/api/v1/health/ready"; then
    cat "$log_directory/api.log"
    exit 1
fi

HUBCR_CONTROL_PLANE_URL="http://127.0.0.1:${api_port}" bun run --cwd frontend build
HUBCR_CONTROL_PLANE_URL="http://127.0.0.1:${api_port}" \
    bun run --cwd frontend start --hostname 127.0.0.1 --port "$web_port" >"$log_directory/web.log" 2>&1 &
web_pid=$!
if ! wait_for_url "http://127.0.0.1:${web_port}"; then
    cat "$log_directory/web.log"
    exit 1
fi

frontend/node_modules/.bin/playwright test --config frontend/playwright.fullstack.config.ts
