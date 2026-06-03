# P13-T-301 · 真机端到端 E2E + buildx multi-arch push

- **Commit**: this commit (main agent)
- **Date**: 2026-06-03
- **Duration**: plan 2.5d vs actual ~0.5d (real-cluster RUN is lab-gated; this lands the harness + the push job + the runbook).

## Intent

Closer for Bucket B+A: (1) the **buildx multi-arch image push** CI job deferred from Phase 12
(ci.yml:334 "Track A 真 CI/CD pipeline"); (2) a **real-machine connection-stamp harness** for the
build-doc §4.4 five real-hardware checks; (3) close the real-profile mock-data gap (topology/deploy now
real). The actual real-cluster run is LAB-GATED (no 910B here) — this lands the runnable artifacts; the
lab run produces the 🟢 stamps recorded in checkpoint-phase13.

## What landed

- **`.github/workflows/release-images.yml`** — multi-arch (linux/amd64,linux/arm64) buildx push to GHCR
  for all 10 service Dockerfiles (demo-backend + 9 operators/exporter). QEMU + buildx + metadata-action
  (lowercases the mixed-case repo path · derives tags) + build-push-action `push: true`.
  - **Trigger deliberately decoupled from the phase CI gate**: `workflow_dispatch` + `push tags v*` only.
    ci.yml (the gate · per memory feedback_post_tag_ci_gate) runs on the **dev push** at the phase tag;
    `phase-13-complete` is a `phase-*` tag, NOT `v*`, and this workflow ignores branch pushes — so a
    heavyweight image push (which I can't exercise from here) can NEVER redden the phase gate. Non-PR-
    blocking by construction. A lab refresh = manual dispatch; a real release = a `v*` tag.
  - **Context correctness (P1 · verified, not guessed)**: matrix `context` = module dir, matching the
    existing `docker build -t … "${REPO_ROOT}/backend"` in tests/e2e/kind/install.sh; the Dockerfiles
    `COPY . .` relative to that + carry `ARG TARGETARCH` + `GOARCH=${TARGETARCH}` (Phase 12 multi-arch ·
    ADR-0020) so each platform cross-compiles CGO-free.
- **`tests/e2e/real/connection-stamps.sh`** + **README.md** — the 5 §4.4 stamps (real NPU discovery /
  real slice / real telemetry / real inference / HCCS affinity), each mapped 1:1 to a §4.4 row + its
  Phase-13 real body (T101/T102/T103/T105). PASS/FAIL/**SKIP** (SKIP = prerequisite absent, not a
  failure → a partial lab still yields a useful report) · exits non-zero on any FAIL. Encodes the plan
  §8 verification split (demo verifies FUNCTION · real verifies CONNECTION) + the single-point fallback
  (a blocked real point → mark THAT point lab-driver-gated · do NOT reopen a phase).
- **`deploy/profiles/real/README.md`** — real-source table flipped all ✅ (topology→k8s GetTopology
  T103 · deploy→k8s Deploy T104 · NPU→real-Ascend T101 · telemetry T102 · authz/secret/quota/HA/SLA
  T201-206); **mock-data gap CLOSED** (topology/deploy real → no `demo-backend-mockdata` ConfigMap
  needed); install command gains `--with-secrets`; "仍属 Phase-13" → "真机端到端验证 (lab-gated · 软件层已完整)".

## Path adaptations (transparent · P3 · M4)

- **No arm64 kind smoke** (the plan listed `tests/e2e/kind/*` "若适用"). GitHub-hosted free runners are
  amd64; an arm64 kind job needs arm64 runner infra the repo doesn't have. Real arm64 verification is the
  lab path (`tests/e2e/real/`). Fabricating an arm64 kind job that can't run = padding (M4) → skipped,
  documented here.
- **release-images.yml is a NEW workflow file** (not an edit to ci.yml). Isolating it keeps the
  image-push side-effect out of the PR/dev gate (the alternative — a job in ci.yml gated on push — would
  run on every dev push including the phase-tag push, risking the gate on an untestable heavyweight job).

## Verification (offline · plan §8 layer 1)

- `release-images.yml` parses (yaml.safe_load · 1 job · 10 matrix images); ci.yml/e2e-kind.yml/
  helm-lint.yml still parse (no accidental edits).
- buildx matrix `context` verified against the existing `docker build` invocation + Dockerfile COPY
  semantics (not guessed).
- `bash -n tests/e2e/real/connection-stamps.sh` clean · `chmod +x`.

**Real-machine (LAB-GATED · the stamp)**: `scripts/install.sh --profile real --all-phase-4
--with-secrets` → all Pods Ready → `bash tests/e2e/real/connection-stamps.sh` (5 §4.4 stamps) +
`tests/sla` P99 → record in checkpoint-phase13 + flip build-doc §4.4 🔴→🟢. The Release Images workflow
(manual dispatch) pushes arm64 images the lab cluster pulls.

## Carry-forward

- **T302**: stamp build-doc §4.4/§5 🔴→🟢/`[x]` (with lab numbers where the lab ran · else "harness
  landed · lab-gated") + checkpoint-phase13 + tag.
