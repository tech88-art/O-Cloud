# Deploy Profile — DEMO edition(验证数据版)

每个资源走 **验证(mock / simulator)数据**,不需真硬件。用于功能演示 / 回归。

## 这版是什么
- 后端:全 mock(`backend/configs/config.dev.yaml`)。
- npu-dra-driver:`mock-json` 发布者(内置 `npus.json`)。
- exporter:`simulator.enabled=true`(sim.json),`nodeSelector` 清空,任意节点可调度。
- inference-operator:metrics degraded(无真 Prometheus)。

## 怎么选这版验证
- **最常用(整栈)**:`scripts/install.sh`(docker-compose · `make run` 同源)→ http://localhost:3000。
- **K8s helm**:`scripts/install.sh --profile demo --all-phase-4`,或对各 chart:
  ```bash
  helm upgrade --install <chart> deploy/helm-charts/<chart> \
    -n <ns> -f deploy/profiles/demo/<chart>.values.yaml
  ```

> 真硬件版见 `../real/README.md`。两版**只差这套 profile 值 + 后端 config 文件**,
> 单一代码库、运行时数据源抽象切换(无 fork)。详见
> `docs/build-and-production-validation.md` §0.1。
