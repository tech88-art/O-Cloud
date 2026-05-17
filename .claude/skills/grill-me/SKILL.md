# grill-me

**Addresses failure mode**: Misalignment — building the wrong thing. See `~/.claude/CODING.md` § 1.

For O-Cloud platform feature requests where requirements are ambiguous about the *module*, the *task scope*, the *domain terminology*, or the *acceptance criteria*.

## When to use

User (or task package) says things like "add support for X", "make it faster", "wire up Y", "add endpoint Z", "do AI-RAN" — without specifying: target module, allowed paths, exact API/CRD/schema fields, dependency status, or pinned acceptance criteria. Also fire whenever a request crosses a domain term whose meaning splits (see below).

## Process

1. **Explore first.** Before asking the user anything, grep the repo. If the answer is in:
   - `docs/architecture.md` — read relevant section
   - `docs/agent-coordination.md` §0a — single-agent execution rules
   - `<module>/CLAUDE.md` — module-specific conventions
   - `docs/api-contract.yaml` — REST/WS contracts
   - `configs/mock-data/schema.json` — Mock data shape
   - `operators/pool-operator/api/v1alpha1/*.go` — CRD field definitions
   - `docs/adr/*.md` — historical decisions

   then answer from code, do not ask.

2. **One question at a time.** No question lists. Wait for the answer before sending the next.

3. **Pre-coding checklist** (O-Cloud project):
   - **Module**: backend / frontend / operators / configs / deploy / docs?
   - **Task package**: P1-T-XXX (if assigned) or pending creation?
   - **Allowed Paths**: explicit white-list from the task package
   - **Forbidden Paths**: especially shared contracts (`docs/api-contract.yaml`, `configs/mock-data/schema.json`, CRD types) — those go through chat + ADR (`agent-coordination.md §0a.5`), never inline change
   - **Dependencies**: `Depends on` tasks all merged to `dev`?
   - **Acceptance Criteria**: machine-verifiable list, no fuzzy "looks good"
   - **Domain disambiguation** — if the request crosses any of the terms below, pin which sense

4. **Required domain disambiguation** (O-Cloud):
   - **"NPU"** alone is meaningless → which vendor / SKU / driver? (Ascend 910B is default)
   - **"DRA" vs "Device Plugin"** → which K8s resource model? (DRA is Beta in K8s 1.31, Phase 4 primary is Device Plugin per ADR-0001 修订)
   - **"AI-for-RAN vs AI-RAN vs AI-on-RAN"** → three distinct things (TRL gap ≥3, CTO decisions opposite)
   - **"Edge"** → cell-site (gNB/DU/RU) vs aggregation (CU/MEC) vs device
   - **"Pool"** → ClusterPool / NodePool / NPUPool / NPUSlicePool — which level?
   - **"Slice"** → CRD `NPUSlice` instance vs `NPUSlicePool` definition vs `SliceTemplate`?
   - **"PD 分离"** → vLLM disaggregation vs llm-d vs MindIE?
   - **"O2"** → O-RAN O2 IMS or DMS interface? Which version?

5. **Restate the spec, get a yes, then code.**

## Anti-patterns

- Asking five questions at once.
- Asking "which module?" when the task package header states it.
- Coding before Acceptance Criteria are machine-verifiable.
- Accepting "wire it up" without endpoint contract details.
- Editing a shared contract file inline instead of chat + ADR.
- Accepting "NPU" / "Pool" / "Edge" without disambiguation.

## Why this skill exists

Misalignment is the single largest cause of wasted Claude-generated code on multi-module / multi-domain projects. O-Cloud multiplies the surface (5 modules × shared contracts × Ascend/K8s/AI domain terminology). Without an explicit interrogation step, Claude produces code that compiles, lints, tests green — but solves the wrong problem or breaks the contract.
