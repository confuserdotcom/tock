VERSION ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)

.PHONY: build test test-quick vet clean

build:
	CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" -o bin/trackd ./cmd/trackd

test-quick:
	CGO_ENABLED=0 go test ./internal/span/... ./internal/store/... -count=1

test:
	go vet ./...
	go build ./...
	CGO_ENABLED=0 go test ./internal/span/... ./internal/store/... -count=1
	go test ./... -count=1

vet:
	go vet ./...

clean:
	rm -rf bin build
