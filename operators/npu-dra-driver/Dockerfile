# Phase 4 T101 multi-stage image build. Mirrors
# operators/pool-operator/Dockerfile + exporters/ascend-npu-exporter-plus
# patterns: golang builder + distroless static runtime.
# multi-arch (ADR-0020 · P12-T-101): aarch64 鲲鹏 + 昇腾 910B 部署 target
# (翻转此前 amd64-only / 鲲鹏-unsupported · 见 docs/adr/0020) ·
# docker buildx build --platform linux/amd64,linux/arm64 · TARGETARCH 由 buildx 注入.
#
# Built by `make docker-build IMG=...`. The helm chart at
# deploy/helm-charts/npu-dra-driver/ references this image by repo+tag.

FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .

# GOARCH=${TARGETARCH} → buildx 跨平台(per ADR-0020 multi-arch · 空=host arch).
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -a -o manager cmd/main.go

# Distroless static minimal runtime.
FROM gcr.io/distroless/static:nonroot

# OCI labels (P4-T-101). Helm chart annotations match these on the
# Deployment template so kubectl describe surfaces the image origin.
LABEL org.opencontainers.image.title="npu-dra-driver"
LABEL org.opencontainers.image.description="O-Cloud Ascend NPU DRA driver (Phase 4 simulator-first; real Ascend integration Phase 5+)"
LABEL org.opencontainers.image.source="https://github.com/tech88-art/O-Cloud"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
