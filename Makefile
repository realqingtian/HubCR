.PHONY: dev-api dev-worker dev-web db-migrate infra-config infra-up infra-down infra-status infra-smoke test test-integration test-m1-e2e check-docs check-secrets check

HUBCR_COMPOSE_FILE ?= deployments/compose/compose.yaml
HUBCR_ENV_FILE ?= .env
HUBCR_REGISTRY_PORT ?= 5000
HUBCR_COMPOSE = docker compose --env-file $(HUBCR_ENV_FILE) -f $(HUBCR_COMPOSE_FILE)

dev-api:
	cd backend && go run ./cmd/api

dev-worker:
	cd backend && go run ./cmd/worker

dev-web:
	cd frontend && bun run dev

db-migrate:
	cd backend && go run ./cmd/migrate

infra-config:
	$(HUBCR_COMPOSE) config --quiet

infra-up:
	$(HUBCR_COMPOSE) up -d

infra-down:
	$(HUBCR_COMPOSE) down

infra-status:
	$(HUBCR_COMPOSE) ps --all

infra-smoke:
	$(HUBCR_COMPOSE) exec -T postgres sh -c 'pg_isready -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
	$(HUBCR_COMPOSE) exec -T redis redis-cli ping
	curl --fail --silent --show-error --output /dev/null --write-out 'MinIO HTTP %{http_code}\n' http://localhost:9000/minio/health/live
	curl --fail --silent --show-error --output /dev/null --write-out 'Registry HTTP %{http_code}\n' http://localhost:$(HUBCR_REGISTRY_PORT)/v2/
	curl --fail --silent --show-error --write-out '\n' http://localhost:$(HUBCR_REGISTRY_PORT)/v2/_catalog

test:
	cd backend && go test ./...
	cd frontend && bun run test

test-integration:
	sh scripts/backend-integration.sh

test-m1-e2e:
	sh scripts/m1-e2e.sh

check-docs:
	python3 scripts/check-docs.py

check-secrets:
	python3 scripts/check-secrets.py

check: check-docs check-secrets
	cd backend && test -z "$$(gofmt -l .)"
	cd backend && go vet ./...
	cd backend && go test ./...
	cd frontend && bun run typecheck
	cd frontend && bun run test
	cd frontend && bun run lint
	cd frontend && bun run build
