.PHONY: run test test-integration test-all test-endpoints fmt tidy docker-build docker-up docker-down

run:
	go run ./cmd/server

test:
	go test ./internal/... -v

test-integration:
	go test ./test/integration/... -v -tags=integration

test-all: test test-integration

test-endpoints:
	powershell -ExecutionPolicy Bypass -File ./scripts/test-all-endpoints.ps1

fmt:
	gofmt -w cmd internal pkg test

tidy:
	go mod tidy

docker-build:
	docker build -t go-file-explorer:latest .

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down