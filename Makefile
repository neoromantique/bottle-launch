.PHONY: build clean fmt lint vet check all

# Binary name
BINARY := bottle-launch

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

all: check build

build:
	$(GOBUILD) -o $(BINARY) .

clean:
	$(GOCLEAN)
	rm -f $(BINARY)

fmt:
	$(GOFMT) ./...

vet:
	$(GOVET) ./...

lint: fmt vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

check: fmt vet
	@echo "Checking format..."
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)
	@echo "All checks passed!"
