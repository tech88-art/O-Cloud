# syntax=docker/dockerfile:1.7
# Multi-stage build for ascend-npu-exporter-plus.
# Phase 3 scope: skeleton + build_info metric. Collectors land in T007+.
#
# multi-arch (ADR-0020 · P12-T-101): exporter 跑在 aarch64 鲲鹏 NPU 节点上,
# 故必须 arm64(翻转此前 linux/amd64-only · 见 exporters/CLAUDE.md §3.3/§7 已同步)。
#   docker buildx build --platform linux/amd64,linux/arm64 -t ascend-npu-exporter-plus .
#   TARGETARCH 由 buildx 注入 · CGO_ENABLED=0 静态跨编译 · golang-alpine + distroless 均 multi-arch.

FROM golang:1.24-alpine AS build

ARG VERSION=0.1.0
ARG COMMIT=""
ARG BUILDTIME=""
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w \
      -X github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version.Version=${VERSION} \
      -X github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version.Commit=${COMMIT} \
      -X github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version.BuildTime=${BUILDTIME}" \
    -o /out/exporter-plus ./cmd/exporter-plus

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/exporter-plus /app/exporter-plus
EXPOSE 9100
USER nonroot:nonroot
ENTRYPOINT ["/app/exporter-plus"]
CMD ["--listen=:9100"]
