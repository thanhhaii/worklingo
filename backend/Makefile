export PATH := $(PATH):/usr/local/go/bin:$(HOME)/go/bin

.PHONY: run build migrate sqlc test vet smoke tools

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

migrate:
	go run ./cmd/api migrate

sqlc:
	sqlc generate

test:
	go test ./...

vet:
	go vet ./...

tools:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Requires a running server (ideally with short timings, see README).
smoke:
	go run ./scripts/smoke
