# ascend-npu-exporter-plus

O-Cloud 自建 NPU Prometheus exporter,替代 Phase 2 安装的社区
`ascend-npu-exporter v6.0.0` chart + dev stub。

详细模块约束见 [`exporters/CLAUDE.md`](../CLAUDE.md);Phase 3 路线见
[`docs/phase3-plan.md`](../../docs/phase3-plan.md)。

## Phase 3 现状

| Task | 状态 | 范围 |
|---|---|---|
| **P3-T-006** | done | skeleton:`/metrics` + `/healthz` + `exporter_build_info` + `exporter_collect_duration_seconds` |
| P3-T-007 | todo | NPU-level collector(`ascend_npu_*`) + simulator JSON 接入 |
| P3-T-101 | todo | slice-level collector(`ascend_slice_*`) |
| P3-T-102 | todo | PID-level collector(`ascend_pid_*`),启用需 `--enable-workload-correlation` |
| P3-T-103 | todo | 自建 Helm chart + 下线社区 chart + dev stub |

Phase 3 全程 simulator-only;DCMI / `npu-smi` 真硬件接入推迟到 Phase 4+
(`internal/collector/sources/dcmi.go` + `npu_smi.go`)。

## Quick start

```sh
cd exporters/ascend-npu-exporter-plus
make build-local
./bin/exporter-plus.local --listen=:9100
curl http://localhost:9100/metrics | grep exporter_build_info
curl http://localhost:9100/healthz   # -> ok
```

## CLI flags

- `--listen` — `/metrics` HTTP 监听地址,默认 `:9100`
- `--simulator` — 模拟器 JSON 路径,空 = DCMI(Phase 4+ 才接;Phase 3 stub
  收下但忽略)

## 构建变体

```sh
make build          # linux/amd64 (Docker / cluster) -> bin/exporter-plus
make build-local    # host OS                         -> bin/exporter-plus.local
make docker-build   # linux/amd64 distroless image
make test           # go test ./... + cover.out
make lint           # golangci-lint
make clean          # remove bin/ + cover.out
```

`-ldflags` 注入版本元数据到 `internal/version`,通过
`exporter_build_info{version,commit,go_version}` 暴露。
