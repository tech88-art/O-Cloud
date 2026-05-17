# syntax=docker/dockerfile:1.7
#
# Multi-stage build for demo-backend.
#   stage 1 (build)   : compile a static binary against the Go toolchain.
#   stage 2 (runtime) : distroless static image — minimal attack surface,
#                       no shell, no package manager.

# ---- build stage ----------------------------------------------------------
FROM golang:1.24-alpine AS build

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
