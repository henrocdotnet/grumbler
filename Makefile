.PHONY: build build-shared install test test-seq test-live test-live-claude test-live-gemini test-live-codex test-live-anthropic test-live-google test-integration test-project-reset lint format clean

BINARY := grumbler
BUILD_DIR := ./bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/grumbler

build-shared:
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BINARY) ./cmd/grumbler

install: build
	go install ./cmd/grumbler

test:
	go test ./...

# Run each package's tests sequentially with a timeout, to surface hangs.
test-seq:
	go test -v -count=1 -timeout 30s github.com/henrocdotnet/grumbler/internal/config
	go test -v -count=1 -timeout 30s github.com/henrocdotnet/grumbler/internal/model
	go test -v -count=1 -timeout 30s github.com/henrocdotnet/grumbler/internal/rules
	go test -v -count=1 -timeout 30s github.com/henrocdotnet/grumbler/internal/git
	go test -v -count=1 -timeout 30s github.com/henrocdotnet/grumbler/internal/output
	go test -v -count=1 -timeout 30s github.com/henrocdotnet/grumbler/internal/llm
	go test -v -count=1 -timeout 30s github.com/henrocdotnet/grumbler/internal/pipeline/stages
	go test -v -count=1 -timeout 30s github.com/henrocdotnet/grumbler/internal/pipeline


test-live:
	go test -v -count=1 -timeout 120s -tags live github.com/henrocdotnet/grumbler/internal/llm

INTEGRATION_TIMEOUT ?= 10m

test-integration:
	INTEGRATION_TIMEOUT=$(INTEGRATION_TIMEOUT) go test -v -count=1 -tags integration github.com/henrocdotnet/grumbler/internal/integration

# Delete the synthetic test project so it gets recreated on the next test-integration run.
test-project-reset:
	rm -rf .test-project

test-live-claude:
	go test -v -count=1 -timeout 120s -tags live -run TestLiveClaudeCLI github.com/henrocdotnet/grumbler/internal/llm

test-live-gemini:
	go test -v -count=1 -timeout 120s -tags live -run TestLiveGeminiCLI github.com/henrocdotnet/grumbler/internal/llm

test-live-codex:
	go test -v -count=1 -timeout 120s -tags live -run TestLiveCodexCLI github.com/henrocdotnet/grumbler/internal/llm

test-live-anthropic:
	go test -v -count=1 -timeout 120s -tags live -run TestLiveAnthropicAPI github.com/henrocdotnet/grumbler/internal/llm

test-live-google:
	go test -v -count=1 -timeout 120s -tags live -run TestLiveGoogleAPI github.com/henrocdotnet/grumbler/internal/llm

format:
	gofmt -l -w ./cmd ./internal

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR)
