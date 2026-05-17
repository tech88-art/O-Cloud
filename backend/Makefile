# O-Cloud demo-backend — Makefile
#
# Targets:
#   tidy   : go mod tidy
#   build  : build cmd/demo-backend → bin/demo-backend
#   run    : build then start with configs/config.dev.yaml
#   test   : go test ./... with coverage
#   lint   : golangci-lint run (installs to GOPATH/bin if absent)
#   fmt    : gofmt -s -w
#   docker : multi-stage build → ocloud-edge/demo-backend:dev
#
# Override:
#   VERSION   default 0.1.0           — embedded via -ldflags
#   COMMIT    derived from `git rev-parse --short HEAD` (empty if not a repo)
#   BUILDTIME derived from `date -u +%Y-%m-%dT%H:%M:%SZ`

SHELL          := /usr/bin/env bash
MODULE         := github.com/example/ocloud-edge/backend
BINARY         := demo-backend
BIN_DIR        := bin
CMD            := ./cmd/$(BINARY)
DOCKER_IMAGE   ?= ocloud-edge/demo-backend
DOCKER_TAG     ?= dev

VERSION   ?= 0.1.0
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "")
BUILDTIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "")

LDFLAGS := -ldflags "\
	-s -w \
	-X $(MODULE)/pkg/api.BuildCommit=$(COMMIT) \
	-X $(MODULE)/pkg/api.BuildTimestamp=$(BUILDTIME)"

GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null)

.PHONY: help tidy build run test lint fmt docker clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*##' '{printf "  %-12s %s\n", $$1, $$2}'

tidy: ## go mod tidy
	go mod tidy

build: ## Compile binary into bin/
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) $(CMD)
	@echo "built $(BIN_DIR)/$(BINARY)"

run: build ## Run with configs/config.dev.yaml
	./$(BIN_DIR)/$(BINARY) --config configs/config.dev.yaml

test: ## Unit tests + coverage summary
	go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -n 1

test-race: ## Tests with -race (needs CGO; CI Linux job)
	CGO_ENABLED=1 go test -race ./...

lint: ## golangci-lint run
ifeq ($(GOLANGCI_LINT),)
	@echo "installing golangci-lint to GOPATH/bin"
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
endif
	golangci-lint run ./...

fmt: ## gofmt -s -w
	gofmt -s -w .

docker: ## Build container image
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) coverage.out
