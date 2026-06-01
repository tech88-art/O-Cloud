# syntax=docker/dockerfile:1.7
#
# Multi-stage build for demo-backend.
#   stage 1 (build)   : compile a static binary against the Go toolchain.
#   stage 2 (runtime) : distroless static image — minimal attack surface,
#                       no shell, no package manager.

# ---- build stage ----------------------------------------------------------
# P5-T-117 (2026-05-20): bumped golang:1.24-alpine → golang:1.26-alpine.
# `backend/go.mod` declares `go 1.26.3` (the Go ≥ 1.21 toolchain
# directive is enforced at build time), so the previous 1.24 image
# fails `go mod download` with:
#   go: go.mod requires go >= 1.26.3 (running go 1.24.13; GOTOOLCHAIN=local)
# That broke the kind-smoke `build-images` step on commit 61d6a49.
# `1.26-alpine` resolves to the latest 1.26.x patch (≥ 1.26.3).
FROM golang:1.26-alpine AS build

# Build-time metadata baked into the binary via -ldflags.
ARG VERSION=0.1.0
ARG COMMIT=""
ARG BUILDTIME=""

# multi-arch (ADR-0020 · P12-T-101): aarch64 鲲鹏 + 昇腾 910B 部署 target.
#   docker buildx build --platform linux/amd64,linux/arm64 -t demo-backend .
#   TARGETARCH 由 buildx 注入(default docker build → 空 → host arch).
#   CGO_ENABLED=0 静态跨编译 · golang-alpine + distroless/static 均 multi-arch base.
#   arm64 = 部署 target · amd64 保留本机 dev/CI/render-verify(make dev-up 不破).
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Resolve modules first so the layer can be cached when only sources change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO disabled → static binary that runs on distroless/static.
# GOARCH=${TARGETARCH} → buildx cross-compiles per target platform (empty under
# plain `docker build` → host arch, so make dev-up amd64 build is unchanged).
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w \
      -X github.com/example/ocloud-edge/backend/pkg/api.BuildCommit=${COMMIT} \
      -X github.com/example/ocloud-edge/backend/pkg/api.BuildTimestamp=${BUILDTIME}" \
    -o /out/demo-backend ./cmd/demo-backend

# ---- runtime stage --------------------------------------------------------
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

COPY --from=build /out/demo-backend /app/demo-backend
COPY configs/config.example.yaml /app/configs/config.yaml

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/demo-backend"]
CMD ["--config", "/app/configs/config.yaml"]
