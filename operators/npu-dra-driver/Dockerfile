# Phase 4 T003 scaffold stub. Real multi-stage build lands in P4-T-101
# (deploy/helm-charts/npu-dra-driver/ + distroless runtime).
#
# This stub mirrors operators/pool-operator/Dockerfile so `make docker-build`
# in T003 already works (single image, no chart yet).

FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .

# Build for linux/amd64 only (arch §1.2: no arm64 / 鲲鹏 support).
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -a -o manager cmd/main.go

# Distroless static minimal runtime.
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
