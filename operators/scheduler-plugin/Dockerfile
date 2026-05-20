# Phase 6 T002 multi-stage image build. Mirrors operators/npu-dra-driver/
# Dockerfile + pool-operator/Dockerfile patterns: golang builder + distroless
# static runtime, linux/amd64 only (arch §1.2: no arm64 / 鲲鹏 support).
#
# Built by `make docker-build IMG=...`. The helm chart at
# deploy/helm-charts/scheduler-plugin/ (P6-T-101) references this image by
# repo+tag and wraps it as a Deployment running kube-scheduler with the
# custom plugins linked in (ADR-0010 §1).
#
# Binary name is `kube-scheduler` (NOT `manager`) because scheduler-plugin
# IS kube-scheduler — just with our HCCSTopology + NumaAffinity + Binpack
# plugins compiled in. Helm chart deployment.yaml passes `--config=...` to
# consume KubeSchedulerConfiguration ConfigMap.

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
    go build -a -o kube-scheduler cmd/main.go

# Distroless static minimal runtime.
FROM gcr.io/distroless/static:nonroot

# OCI labels (P6-T-002). Helm chart annotations match these on the
# Deployment template so kubectl describe surfaces the image origin.
LABEL org.opencontainers.image.title="scheduler-plugin"
LABEL org.opencontainers.image.description="O-Cloud custom kube-scheduler with HCCSTopology + NumaAffinity + Binpack plugins (Phase 6 scaffold; Filter/Score bodies P6-T-004..T007)"
LABEL org.opencontainers.image.source="https://github.com/tech88-art/O-Cloud"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder /workspace/kube-scheduler .
USER 65532:65532

ENTRYPOINT ["/kube-scheduler"]
