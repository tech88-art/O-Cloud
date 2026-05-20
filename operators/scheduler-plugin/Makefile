# operators/scheduler-plugin Makefile (Phase 6 T002 scaffold).
#
# Mirrors operators/npu-dra-driver/Makefile structure but DROPS Kubebuilder-
# specific targets (controller-gen / manifests / generate / setup-envtest) —
# scheduler-plugin is NOT a Kubebuilder project. It's a custom kube-scheduler
# binary that registers HCCSTopology + NumaAffinity + Binpack plugins via the
# upstream `sigs.k8s.io/scheduler-plugins` framework (ADR-0010 §1).
#
# Binary naming convention: `bin/kube-scheduler` (NOT `bin/manager`). The
# downstream helm chart (P6-T-101) wraps this as a Deployment running with
# `--config=/etc/scheduler/config.yaml` consuming the KubeSchedulerConfiguration
# ConfigMap.

IMG ?= scheduler-plugin:latest
YEAR ?= $(shell date +%Y)

ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CONTAINER_TOOL ?= docker

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests. (Phase 6 T002 scaffold: empty plugin pkgs return 0 tests but build clean. T004-T008 fill in.)
	go test ./... -coverprofile cover.out

.PHONY: tidy
tidy: ## Run go mod tidy.
	go mod tidy

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

##@ Build

.PHONY: build
build: fmt vet ## Build kube-scheduler binary with custom plugins linked in.
	go build -o bin/kube-scheduler cmd/main.go

.PHONY: run
run: fmt vet ## Run scheduler from host (consumes ~/.kube/config; informational — production uses helm chart).
	go run ./cmd/main.go --help

.PHONY: docker-build
docker-build: ## Build docker image with the scheduler.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the scheduler.
	$(CONTAINER_TOOL) push ${IMG}

##@ Dependencies

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.11.4

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef
