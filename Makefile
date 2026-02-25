.PHONY: build install test lint format clean

BINARY := grumbler
BUILD_DIR := ./bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/grumbler

install:
	go install ./cmd/grumbler

test:
	go test ./...

format:
	gofmt -l -w ./cmd ./internal

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR)
