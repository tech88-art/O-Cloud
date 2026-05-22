# P11-T-004 chart build Dockerfile · ADR-0017 §2 Decision D 2nd 优先级
# Multi-stage golang + distroless static · linux/amd64 only per arch §1.2
# (same pattern as inference-operator / npu-dra-driver / pool-operator).

FROM golang:1.26 AS builder
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
LABEL org.opencontainers.image.title="node-lifecycle-operator"
LABEL org.opencontainers.image.description="O-Cloud IMS-1 NodeLifecycle controller (Phase 11 chart packaging per ADR-0003 v2 + ADR-0017 §2 Decision D 2nd)"
LABEL org.opencontainers.image.source="https://github.com/tech88-art/O-Cloud"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
