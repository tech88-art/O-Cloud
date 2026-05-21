# O2 DMS Adapter (P9-T-008 scaffold). Multi-stage Go build · non-root
# runtime · scratch base · port 8088 EXPOSE per ADR-0013 §2 Decision B.

FROM golang:1.26 AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /workspace/bin/o2-dms-adapter ./cmd

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/bin/o2-dms-adapter /usr/local/bin/o2-dms-adapter
EXPOSE 8088
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/o2-dms-adapter"]
