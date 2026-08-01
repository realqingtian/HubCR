.PHONY: dev-api dev-worker dev-web db-migrate registry-dev-keys infra-config infra-up infra-down infra-status infra-smoke test test-integration test-m1-e2e test-m2-registry-e2e check-docs check-secrets check

HUBCR_COMPOSE_FILE ?= deployments/compose/compose.yaml
HUBCR_ENV_FILE ?= .env
HUBCR_REGISTRY_PORT ?= 5000
HUBCR_MINIO_PORT ?= 9000
HUBCR_REGISTRY_AUTH_DIR ?= $(CURDIR)/.data/registry-auth
HUBCR_REGISTRY_AUTH_ENABLED ?= true
HUBCR_REGISTRY_EXTERNAL_URL ?= http://localhost:$(HUBCR_REGISTRY_PORT)
HUBCR_REGISTRY_ALLOW_INSECURE_HTTP ?= true
HUBCR_REGISTRY_SERVICE ?= hubcr-registry
HUBCR_REGISTRY_ISSUER ?= hubcr-token-service
HUBCR_REGISTRY_TOKEN_TTL ?= 5m
HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE ?= $(HUBCR_REGISTRY_AUTH_DIR)/private.pem
HUBCR_REGISTRY_TOKEN_JWKS_FILE ?= $(HUBCR_REGISTRY_AUTH_DIR)/jwks.json
HUBCR_REGISTRY_EVENT_TOKEN ?= hubcr-registry-event-dev-only-000000000000
HUBCR_COMPOSE = HUBCR_REGISTRY_AUTH_DIR="$(HUBCR_REGISTRY_AUTH_DIR)" docker compose --env-file $(HUBCR_ENV_FILE) -f $(HUBCR_COMPOSE_FILE)

dev-api: registry-dev-keys
	cd backend && \
		HUBCR_REGISTRY_AUTH_ENABLED="$(HUBCR_REGISTRY_AUTH_ENABLED)" \
		HUBCR_REGISTRY_EXTERNAL_URL="$(HUBCR_REGISTRY_EXTERNAL_URL)" \
		HUBCR_REGISTRY_ALLOW_INSECURE_HTTP="$(HUBCR_REGISTRY_ALLOW_INSECURE_HTTP)" \
		HUBCR_REGISTRY_SERVICE="$(HUBCR_REGISTRY_SERVICE)" \
		HUBCR_REGISTRY_ISSUER="$(HUBCR_REGISTRY_ISSUER)" \
		HUBCR_REGISTRY_TOKEN_TTL="$(HUBCR_REGISTRY_TOKEN_TTL)" \
		HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE="$(HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE)" \
		HUBCR_REGISTRY_TOKEN_JWKS_FILE="$(HUBCR_REGISTRY_TOKEN_JWKS_FILE)" \
		HUBCR_REGISTRY_EVENT_TOKEN="$(HUBCR_REGISTRY_EVENT_TOKEN)" \
		go run ./cmd/api

dev-worker:
	cd backend && go run ./cmd/worker

dev-web:
	cd frontend && bun run dev

db-migrate:
	cd backend && go run ./cmd/migrate

registry-dev-keys:
	cd backend && go run ./cmd/registry-keygen --output-dir "$(HUBCR_REGISTRY_AUTH_DIR)"

infra-config:
	$(HUBCR_COMPOSE) config --quiet

infra-up: registry-dev-keys
	$(HUBCR_COMPOSE) up -d

infra-down:
	$(HUBCR_COMPOSE) down

infra-status:
	$(HUBCR_COMPOSE) ps --all

infra-smoke:
	$(HUBCR_COMPOSE) exec -T postgres sh -c 'pg_isready -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
	$(HUBCR_COMPOSE) exec -T redis redis-cli ping
	curl --fail --silent --show-error --output /dev/null --write-out 'MinIO HTTP %{http_code}\n' http://localhost:$(HUBCR_MINIO_PORT)/minio/health/live
	test "$$(curl --silent --output /dev/null --write-out '%{http_code}' http://localhost:$(HUBCR_REGISTRY_PORT)/v2/)" = 401
	curl --silent --dump-header - --output /dev/null http://localhost:$(HUBCR_REGISTRY_PORT)/v2/ | grep -Fqi 'www-authenticate: Bearer realm="http://localhost:$(HUBCR_REGISTRY_PORT)/token",service="$(HUBCR_REGISTRY_SERVICE)"'
	@echo 'Registry HTTP 401 with scoped Bearer challenge'

test:
	cd backend && go test ./...
	cd frontend && bun run test

test-integration:
	sh scripts/backend-integration.sh

test-m1-e2e:
	sh scripts/m1-e2e.sh

test-m2-registry-e2e:
	sh scripts/m2-registry-e2e.sh

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
