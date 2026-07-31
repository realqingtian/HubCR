.PHONY: dev-api dev-worker dev-web test check

dev-api:
	cd backend && go run ./cmd/api

dev-worker:
	cd backend && go run ./cmd/worker

dev-web:
	cd frontend && bun run dev

test:
	cd backend && go test ./...
	cd frontend && bun run test

check:
	cd backend && test -z "$$(gofmt -l .)"
	cd backend && go vet ./...
	cd backend && go test ./...
	cd frontend && bun run typecheck
	cd frontend && bun run test
	cd frontend && bun run lint
	cd frontend && bun run build
