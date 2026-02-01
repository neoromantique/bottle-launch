.PHONY: build clean fmt lint vet check all

# Binary name
BINARY := bottle-launch

# Source directory
SRCDIR := src

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

all: check build

build:
	cd $(SRCDIR) && $(GOBUILD) -o ../$(BINARY) .

clean:
	cd $(SRCDIR) && $(GOCLEAN)
	rm -f $(BINARY)

fmt:
	cd $(SRCDIR) && $(GOFMT) ./...

vet:
	cd $(SRCDIR) && $(GOVET) ./...

lint: fmt vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		cd $(SRCDIR) && golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

check: fmt vet
	@echo "Checking format..."
	@cd $(SRCDIR) && test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)
	@echo "All checks passed!"
