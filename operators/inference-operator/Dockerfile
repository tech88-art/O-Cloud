# Phase 4 T103 scaffold Dockerfile. Real chart build lands Phase 5 alongside
# the controller body.
#
# Multi-stage golang + distroless static runtime. multi-arch (ADR-0020 ·
# P12-T-101): aarch64 鲲鹏 + 昇腾 910B 部署 target(翻转此前 amd64-only) ·
# buildx --platform linux/amd64,linux/arm64. Identical pattern to
# operators/npu-dra-driver/Dockerfile + operators/pool-operator/Dockerfile.

FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -a -o manager cmd/main.go

FROM gcr.io/distroless/static:nonroot
LABEL org.opencontainers.image.title="inference-operator"
LABEL org.opencontainers.image.description="O-Cloud inference-operator (Phase 4 scaffold; ModelService controller Phase 5 per ADR-0008 + ADR-0009)"
LABEL org.opencontainers.image.source="https://github.com/tech88-art/O-Cloud"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
