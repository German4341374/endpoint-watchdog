GO ?= go
BINARY := bin/endpoint-watchdog

.PHONY: setup fmt fmt-check vet test build run docker-build clean

setup:
	$(GO) mod download
	$(GO) mod verify

fmt:
	$(GO) fmt ./...

fmt-check:
	test -z "$$(gofmt -l .)"

vet:
	$(GO) vet ./...

test:
	$(GO) test -race -cover ./...

build:
	$(GO) build -trimpath -o $(BINARY) ./cmd/watchdog

run:
	$(GO) run ./cmd/watchdog -config config.yaml

docker-build:
	docker build --tag endpoint-watchdog:local .

clean:
	rm -rf bin coverage.out

