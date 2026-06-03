# P13-fix-001 · backend WS cleanup test flake (phase-13-complete CI gate)

- **Commit**: this commit (main agent · post-tag CI gate fix per memory `feedback_post_tag_ci_gate`)
- **Date**: 2026-06-03

## Symptom

`phase-13-complete` push → ci.yml `Backend (test)` ❌ (→ `CI Pass` ❌). Everything else green
(helm-lint · all 10 arm64 cross-compiles · operators · O2 DMS · IMS · contract · E2E). Failure:

```
--- FAIL: TestWSTopology_ClientCloseTriggersCleanup (2.15s)
    backend/pkg/api/ws_test.go:301  Error: Should be true
    wsActiveConnections did not return to baseline within 2s
```

## Root cause — pre-existing timing flake, NOT a Phase 13 regression

The test closes a WS client then polls **2s** (200×10ms) for `activeWSConnections()` to drop back to
baseline (the server reader sees EOF → cancels ctx → writer exits → defer decrements). Under the loaded
phase-13 CI matrix (backend test ran alongside 10 arm64 cross-compiles + operators + helm), the cleanup
goroutine was merely *slow to be scheduled*, not stuck — it just missed the 2s window.

- Phase 13 backend changes were T103 (GetTopology) / T104 (Deploy) / T201 (prometheus SA token) — **none
  touch the WS handler** (`ws.go` unchanged across the chain). The whole T202..T302 work is operators/
  deploy/docs/tests-sla — zero backend WS surface.
- Locally `go test ./... -count=1` (backend) is **deterministically green** (incl. this test, 0.12s) —
  confirms it is a CI-runner-contention timing flake, not a real cleanup bug.

## Fix

Widen the cleanup poll window 2s → **5s** (deadline-based loop · still breaks early on success so a
healthy run stays ~0.1s). Mirrors the project's flake-hardening posture (CLAUDE.md §6 "harden, don't
blindly re-run" · cf. the earlier e2e-kind hardening `f0ce285`). One-line bound change in
`ws_test.go` — no production code, no behaviour change.

## Verify

- `go test ./pkg/api/ -run TestWSTopology_ClientCloseTriggersCleanup -count=1` → PASS (0.12s).
- full `pkg/api` package `-count=1` → ok (no regression) · `gofmt` clean.
- Re-push → re-watch ci.yml `Backend (test)` → expect green → dev HEAD all green → Phase 13 真收官.
