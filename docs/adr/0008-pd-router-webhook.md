# ADR-0008: PD Router 走 K8s mutating admission webhook(设计 only · 实施 Phase 5)

- **状态**:Accepted (design only — implementation Phase 5)(2026-05-19 — Phase 3 P3-T-106)
- **日期**:2026-05-19
- **决策者**:协调者(用户)
- **相关**:ADR-0002(no-KServe)/ ADR-0005(Pod-in-topology fusion · `npu.huawei.com/slice-bindings` annotation 由 P2-T-105 read path 消费)/ ADR-0006(wire type enum regen)/ phase3-plan.md §P3-T-106 / checkpoint-phase2.md §6 candidate #3 / architecture.md §10.4(inference-operator Phase 5)/ §13(review table)

---

## 上下文

Phase 2 P2-T-105 落地了 `npu.huawei.com/slice-bindings` annotation 的 **read path** — backend `pkg/datasource/k8s` 在列 Workloads 时解析该 annotation,把 slice 绑定信息填到 `model.Pod.Bindings`(详见 ADR-0005)。**写入(mutation)路径**留给 Phase 3+。

`checkpoint-phase2.md §6` 把它列为 Phase 3 候选 #3:

> 3. PD Router admission webhook tied to `npu.huawei.com/slice-bindings`
>    (P2-T-105 parser is the read path; Phase 3 adds the mutation path).

**问题陈述**:vllm-ascend Deployment 创建 Pod 时,如何把"哪个 Pod 跑在哪几个 slice 上"的决策**写**到 Pod 的 annotations 里?这个决策由 inference-operator(Phase 5)的 PD Router 子组件做出(reads ModelService CRD + NPUSlicePool capacity → 选 prefill/decode slice 集合 → 注入 Pod 跑在指定 NPU)。问题转化为:**inference-operator 怎么让自己的 Pod 携带 `npu.huawei.com/slice-bindings` annotation 进入 scheduler?**

候选方案(4 类,Kubernetes 生态典型):

| 方案 | 何时介入 | 注入位置 | 集成成本 | 适配 PD 分离 | Volcano gang scheduling 兼容 |
|---|---|---|---|---|---|
| **A. K8s 原生 mutating admission webhook** | API server 接收 Pod CREATE 但未持久化前 | 直接改 Pod.metadata.annotations | 中(cert + webhook 配置) | ✓ prefill/decode 用不同绑定 | ✓ webhook 在 Volcano scheduler 之前跑 |
| **B. Controller-side mutation**(Pod 创建后由 inference-operator update) | Pod 已存在(可能已被调度)再 update annotation | API call update Pod | 低(纯 Go controller) | ⚠️ race:Pod 可能已起 vLLM,annotation 改了但 Pod 不重读 | ⚠️ Volcano 已 schedule,annotation 不影响放置 |
| **C. OPA Gatekeeper assign-image / mutation policy** | admission 阶段(类似 A) | 通过 OPA assign policy 改 Pod | 高(引 OPA 全栈 + Rego policy 学曲线) | ⚠️ Rego 难写"动态计算 slice 集合"逻辑(需要查 NPUSlicePool CRD) | ✓ admission 时机正确,但实现复杂 |
| **D. Sidecar / init container approach** | Pod 已起,sidecar / init container 在容器里写 binding 文件 | container env / file,不是 annotation | 低 | ⚠️ Pod 已 scheduled,无法影响放置;只能影响 vLLM 启动逻辑 | ⚠️ 同 B,Volcano 已决策 |

---

## 决策

**选 A:K8s 原生 mutating admission webhook**,由 Phase 5 inference-operator 内置(同 binary)。

理由:

1. **时机正确**:Phase 3 后,Volcano + 自研 NUMA+HCCS scheduler-plugin(Phase 6)需要在 schedule 决策**之前**就看到完整 `npu.huawei.com/slice-bindings` annotation,才能做 NUMA / HCCS 亲和打分。Mutating webhook 是 K8s 唯一原生支持"在持久化前改 Pod"的扩展点;方案 B / D 都太晚。
2. **逻辑可表达**:slice 选取需要 `client.Get(NPUSlicePool)` + 检查 `Status.AvailableSlices` + 按 PD role(`prefill` / `decode` / `single`)挑模板 + 写 annotation。这类逻辑用 Go(webhook handler)直接写,比 Rego(OPA)直观。
3. **与 P2-T-105 read path 对齐**:read path 解析的 annotation key 是 `npu.huawei.com/slice-bindings`,值是 JSON 数组(详见 ADR-0005 + `backend/pkg/datasource/k8s/workloads.go`)。webhook 写同一 key、同一 JSON shape,read / write 一致性自洽。
4. **不引 KServe**:延续 ADR-0002 — vllm-ascend Deployment + 自研 inference-operator 直接管 Pod,不套 `InferenceService` wrapper。webhook 是 inference-operator 自己的能力,不依赖第三方 mesh。
5. **OPA / sidecar 都是 "另一种语言 / 另一个进程"**:Gatekeeper 引入 OPA 全栈 + Rego 团队学曲线,sidecar 是 anti-pattern(改不到调度)。 

---

## 实施(Phase 5 · 与 inference-operator 同 binary)

### Webhook server

```
operators/inference-operator/  (Phase 5 新建)
├── api/v1alpha1/modelservice_types.go   ModelService CRD
├── internal/controller/
│   └── modelservice_controller.go
├── internal/webhook/
│   ├── pdrouter.go                       Pod CREATE mutating handler
│   └── pdrouter_test.go
└── cmd/main.go                            注册 Reconciler + Webhook
```

`PDRouter` 实现 `admission.Handler`(`sigs.k8s.io/controller-runtime/pkg/webhook/admission`):

```go
type PDRouter struct {
    client.Client
    decoder *admission.Decoder
}

func (r *PDRouter) Handle(ctx context.Context, req admission.Request) admission.Response {
    pod := &corev1.Pod{}
    if err := r.decoder.Decode(req, pod); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }
    // 1. 看 Pod owner refs 是否来自 ModelService 控制的 Deployment
    //    (label "ocloud.edge.example.com/model-service": "<name>")
    // 2. 解析 ModelService.Spec.PDRole (prefill / decode / single)
    // 3. Get 对应 NPUSlicePool, 选 available slice 集合
    // 4. 写 pod.Annotations["npu.huawei.com/slice-bindings"] = JSON
    // 5. 返回 admission.PatchResponseFromRaw(原始, 修改后)
    ...
}
```

### cert-manager 依赖

Webhook 需要 TLS。Phase 5 部署时 cert-manager 已就位(见 T104 kind smoke 安装序列)。inference-operator 的 webhook 配置使用 `cert-manager.io/inject-ca-from` annotation 让 cert-manager 自动注入 CA bundle 到 `MutatingWebhookConfiguration`,证书走 `Certificate` CRD。

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: pd-router-mutating-webhook
  annotations:
    cert-manager.io/inject-ca-from: ocloud-system/inference-operator-webhook-cert
webhooks:
- name: pdrouter.ocloud.edge.example.com
  clientConfig:
    service:
      name: inference-operator-webhook
      namespace: ocloud-system
      path: /mutate-v1-pod
  rules:
  - apiGroups: [""]
    apiVersions: [v1]
    operations: [CREATE]
    resources: [pods]
  failurePolicy: Fail
  sideEffects: None
  admissionReviewVersions: [v1]
  objectSelector:
    matchLabels:
      ocloud.edge.example.com/managed-by: inference-operator
```

`objectSelector` 把 webhook 作用域限定在 inference-operator 自己创建的 Pod,避免影响 kube-system / monitoring 等命名空间。

### failurePolicy=Fail

**严格模式**(closed default):若 inference-operator webhook 服务不可用 / 返回错误 → Pod CREATE 被拒绝。

理由:PD 分离推理对"slice 绑定一定要存在"是硬约束;失败回退到 "Pod 起来但跑在 default NPU" 会导致 vLLM 启动失败或 OOM。宁可创建被拒,操作人员看 webhook 日志诊断。

**逃生门**:label `ocloud.edge.example.com/bypass-pd-router: "true"` 可绕过(operator 自查 + 重启 webhook 期间紧急部署)。

### Ordering vs Volcano gang scheduling

K8s admission 流程:
```
client CREATE Pod
    ↓
1. authentication
2. authorization
3. mutating admission webhooks (此处 PD Router 跑 — 注入 annotation)
4. object schema validation
5. validating admission webhooks
6. etcd persist
    ↓
scheduler (Volcano gang scheduling 看完整 Pod incl. annotations)
```

Volcano gang scheduling 在 etcd persist **之后** 看 Pod。所以 webhook 在 etcd persist **之前** 注入的 annotation 一定被 Volcano 看到 → 自研 NUMA+HCCS scheduler-plugin(Phase 6)能用 annotation 做亲和打分。

不存在 ordering 冲突。

### 幂等性合约

Webhook 处理 Pod CREATE,**不**处理 UPDATE(避免重复 mutation 导致 annotation 漂移)。如果 Pod 已有 `npu.huawei.com/slice-bindings` annotation(罕见 — 重建场景或测试),webhook **跳过 mutation 返回 Allowed 不改**。这样 webhook 可以重启 / 升级而不会破坏运行中 Pod。

```go
if _, ok := pod.Annotations[npuSliceBindingsAnnotation]; ok {
    return admission.Allowed("annotation already present, skip mutation")
}
```

### 测试

- **Phase 5 单元测试**:`Handle` 函数模拟 admission.Request,断言 patch 包含正确 JSON
- **Phase 5 集成测试**:kind 集群 + cert-manager + 部署 inference-operator + 创建 ModelService + 看 Pod annotation 实际注入
- **Phase 3 不做**:只 design

### 不做(明示)

- **Phase 3 webhook impl**:本 ADR 只 design,实施 Phase 5(inference-operator 启动时)
- **客户端 SDK**:不为外部 client 暴露"如何手写正确 annotation"的 SDK,annotation 是 internal contract(可演进)
- **Multi-cluster webhook**:Phase 9 Karmada 多站点时再考虑跨 cluster 一致性
- **Webhook metrics / tracing**:Phase 5 落地 admission 调用次数 / 延迟指标,本 ADR 不展开

---

## 后果

### 正面

- Phase 5 inference-operator 落地时,PD Router 的设计已锁定,实施只需写代码不需重新选型
- 与 P2-T-105 read path / ADR-0005 / ADR-0002 (no-KServe) 全部对齐 — Phase 3 留的 hook 在 Phase 5 自然接入
- failurePolicy=Fail + objectSelector 的组合在生产场景下 fail-safe(不影响系统 namespace,自家 Pod 走严格路径)

### 负面 / 风险

- **inference-operator 部署依赖 cert-manager**:Phase 5 install.sh 需要先装 cert-manager;Phase 3 T104 kind smoke 已纳入预安装序列,降低 Phase 5 风险
- **Webhook 单点**:inference-operator down 期间无法创建 PD 分离 Pod;缓解 = HA replica 2 + leader election(controller-runtime 默认支持)
- **annotation 演进风险**:`npu.huawei.com/slice-bindings` JSON shape 未来变更需要 read + write 同步;ADR-0006 wire type enum regen 经验适用 — schema 改动走 RFC

## 推翻条件

- Phase 5 实施时发现 admission webhook 性能(p99 延迟)超过 ~50ms,影响 Pod 创建吞吐 → 重新评估方案 B (controller-side mutation with Pod recreate)
- 客户安全合规拒绝 cert-manager 集群级 CA → 评估方案 C(OPA Gatekeeper · 假设客户已用 OPA)
- K8s 升级到引入新 Pod identity 机制(如 DRA ResourceClaim 自带 NPU 绑定)→ 评估是否 annotation 写入路径被 DRA 标准取代(ADR-0001 §7 DRA migration 落地后重新审视)

---

**END of ADR-0008**
