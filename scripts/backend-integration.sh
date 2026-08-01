#!/bin/sh

set -eu

test_project=hubcr-integration
test_compose=deployments/compose/compose.integration.yaml
test_port=${HUBCR_TEST_POSTGRES_PORT:-55432}

cleanup() {
    HUBCR_TEST_POSTGRES_PORT="$test_port" docker compose --project-name "$test_project" --file "$test_compose" down --volumes --remove-orphans
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

HUBCR_TEST_POSTGRES_PORT="$test_port" docker compose --project-name "$test_project" --file "$test_compose" up --detach --wait
HUBCR_TEST_DATABASE_URL="postgres://hubcr_test:hubcr-test-only@127.0.0.1:${test_port}/hubcr_test?sslmode=disable" \
    go -C backend test ./internal/platform/postgres -count=1
HUBCR_TEST_DATABASE_URL="postgres://hubcr_test:hubcr-test-only@127.0.0.1:${test_port}/hubcr_test?sslmode=disable" \
    go -C backend test ./migrations -count=1
HUBCR_TEST_DATABASE_URL="postgres://hubcr_test:hubcr-test-only@127.0.0.1:${test_port}/hubcr_test?sslmode=disable" \
    go -C backend test ./internal/platform/postgres/authstore -count=1
HUBCR_TEST_DATABASE_URL="postgres://hubcr_test:hubcr-test-only@127.0.0.1:${test_port}/hubcr_test?sslmode=disable" \
    go -C backend test ./internal/platform/postgres/namespacestore -count=1
HUBCR_TEST_DATABASE_URL="postgres://hubcr_test:hubcr-test-only@127.0.0.1:${test_port}/hubcr_test?sslmode=disable" \
    go -C backend test ./internal/platform/postgres/organizationstore -count=1
HUBCR_TEST_DATABASE_URL="postgres://hubcr_test:hubcr-test-only@127.0.0.1:${test_port}/hubcr_test?sslmode=disable" \
    go -C backend test ./internal/platform/httpapi/organizationhandler -count=1
