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

WORKDIR /src

# Resolve modules first so the layer can be cached when only sources change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO disabled → static binary that runs on distroless/static.
RUN CGO_ENABLED=0 GOOS=linux go build \
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
