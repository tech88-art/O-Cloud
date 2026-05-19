# Makefile for ascend-npu-exporter-plus.
#
# `make build`        -> linux/amd64 binary (for Docker / on-cluster deployment)
# `make build-local`  -> host-OS binary (for `make run` and functional smoke)
# `make test`         -> go test ./... with coverage profile
# `make docker-build` -> linux/amd64 distroless image
# `make lint`         -> golangci-lint
# `make run`          -> build-local + execute
# `make clean`        -> remove bin/ and cover.out

BIN_DIR ?= bin
IMG     ?= ascend-npu-exporter-plus:dev
VERSION ?= 0.1.0
COMMIT  := $(shell git rev-parse --short HEAD 2>NUL || echo none)

LDFLAGS := -s -w \
  -X github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version.Version=$(VERSION) \
  -X github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version.Commit=$(COMMIT)

.PHONY: help
help:
	@echo "Targets: build / build-local / test / docker-build / lint / clean / run"

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/exporter-plus ./cmd/exporter-plus

.PHONY: build-local
build-local:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/exporter-plus.local ./cmd/exporter-plus

.PHONY: test
test:
	go test ./... -coverprofile cover.out

.PHONY: docker-build
docker-build:
	docker build --platform=linux/amd64 -t $(IMG) \
	  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) .

.PHONY: lint
lint:
	golangci-lint run

.PHONY: run
run: build-local
	./$(BIN_DIR)/exporter-plus.local

.PHONY: clean
clean:
	@if exist $(BIN_DIR) rmdir /S /Q $(BIN_DIR)
	@if exist cover.out del cover.out
