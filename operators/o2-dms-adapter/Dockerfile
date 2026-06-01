# O2 DMS Adapter (P9-T-008 scaffold). Multi-stage Go build · non-root
# runtime · scratch base · port 8088 EXPOSE per ADR-0013 §2 Decision B.

FROM golang:1.26 AS builder
# multi-arch (ADR-0020 · P12-T-101): aarch64 鲲鹏 + 昇腾 910B 部署 target.
#   docker buildx build --platform linux/amd64,linux/arm64 -t o2-dms-adapter .
#   TARGETARCH 由 buildx 注入 · CGO_ENABLED=0 静态跨编译 · distroless/static multi-arch.
ARG TARGETOS
ARG TARGETARCH
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" \
    -o /workspace/bin/o2-dms-adapter ./cmd

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/bin/o2-dms-adapter /usr/local/bin/o2-dms-adapter
EXPOSE 8088
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/o2-dms-adapter"]
