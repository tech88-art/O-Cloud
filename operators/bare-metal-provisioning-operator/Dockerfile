# P11-T-006 chart build Dockerfile · ADR-0017 §2 Decision D 4th
# Multi-stage golang + distroless static (same pattern as IMS-1 / IMS-2).

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
LABEL org.opencontainers.image.title="bare-metal-provisioning-operator"
LABEL org.opencontainers.image.description="O-Cloud IMS-3 BareMetalNode controller with Redfish/IPMI stub clients (Phase 11 chart packaging per ADR-0003 v2 + ADR-0017 §2 Decision D 4th)"
LABEL org.opencontainers.image.source="https://github.com/tech88-art/O-Cloud"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
