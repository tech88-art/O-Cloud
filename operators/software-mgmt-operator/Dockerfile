# P11-T-005 chart build Dockerfile · ADR-0017 §2 Decision D 3rd
# Multi-stage golang + distroless static (same pattern as IMS-1 chart).

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
LABEL org.opencontainers.image.title="software-mgmt-operator"
LABEL org.opencontainers.image.description="O-Cloud IMS-2 SoftwareBundle controller (Phase 11 chart packaging per ADR-0003 v2 + ADR-0017 §2 Decision D 3rd)"
LABEL org.opencontainers.image.source="https://github.com/tech88-art/O-Cloud"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
