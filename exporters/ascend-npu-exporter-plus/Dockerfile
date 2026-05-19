# syntax=docker/dockerfile:1.7
# Multi-stage build for ascend-npu-exporter-plus.
# Phase 3 scope: skeleton + build_info metric. Collectors land in T007+.

FROM --platform=linux/amd64 golang:1.24-alpine AS build

ARG VERSION=0.1.0
ARG COMMIT=""
ARG BUILDTIME=""

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w \
      -X github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version.Version=${VERSION} \
      -X github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version.Commit=${COMMIT} \
      -X github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version.BuildTime=${BUILDTIME}" \
    -o /out/exporter-plus ./cmd/exporter-plus

FROM --platform=linux/amd64 gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/exporter-plus /app/exporter-plus
EXPOSE 9100
USER nonroot:nonroot
ENTRYPOINT ["/app/exporter-plus"]
CMD ["--listen=:9100"]
