.PHONY: all fmt lint build test fuzz coverage

all: fmt lint build test

fmt:
	@echo "🖌️  Formatting: gofmt -w ."
	@gofmt -w .

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "🔍 Linting: golangci-lint run"; \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint not installed, skipping"; \
		echo "   To install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi
	@echo "🔍 Linting: go vet ./..."
	@go vet ./...

build:
	@echo "🏗️  Building: go build ./..."
	@go build ./...

test:
	@echo "🧪 Testing: go test -race ./..."
	@go test -race ./...

fuzz:
	@echo "🌀 Running fuzz tests..."
	go test -fuzz=FuzzParseListLine -fuzztime=10s
	go test -fuzz=FuzzParseFeatures -fuzztime=10s

coverage:
	@echo "📊 Generating coverage report..."
	go test -coverprofile=coverage.out -coverpkg=./... ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report generated at coverage.html"
