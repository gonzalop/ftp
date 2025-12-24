.PHONY: all fmt lint build test fuzz coverage

all: fmt lint build test

fmt:
	@if command -v gofmt >/dev/null 2>&1; then \
		echo "Running gofmt..."; \
		gofmt -w .; \
	else \
		echo "gofmt not installed, skipping"; \
	fi

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

build:
	@echo "Building..."
	go build -v ./...

test:
	@echo "Running tests..."
	go test -v ./...

fuzz:
	@echo "Running fuzz tests..."
	# Fuzz directory listing parser (run for 10 seconds)
	go test -fuzz=FuzzParseListLine -fuzztime=10s
	# Fuzz feature parser (run for 10 seconds)
	go test -fuzz=FuzzParseFeatures -fuzztime=10s

coverage:
	@echo "Generating coverage report..."
	go test -coverprofile=coverage.out -coverpkg=./... ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"
