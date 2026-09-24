.PHONY: dev-db test build lint tidy

dev-db:
	docker compose up -d postgres mailpit

test:
	go test ./... -count=1

build:
	go build -o bin/openforms ./cmd/openforms

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

tidy:
	go mod tidy
