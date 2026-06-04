# P13-fix-003 · build-doc §0.1 stale real-edition boundary (T302 cascade miss)

- **Commit**: this commit (main agent · post-closer docs currency fix)
- **Date**: 2026-06-03

## Symptom

`docs/build-and-production-validation.md` §0.1 (the FIRST demo/real boundary a reader sees) still
carried the **pre-Phase-13** description and **contradicted its own document + the doc it cross-references**:

- line 37: NPU 源 real = `real-ascend`(**Phase-13 stub**)
- line 40: "`topology`+`deploy` 仍走 mock · `real-ascend` 返回 `ErrNotImplemented` · OIDC/Karmada/Vault/
  配额强制/SLA **未实现** · 待 Phase-13 + 真机"

But §4.4 (same doc) says those bodies **land** (🟡), §5 marks all 6 production-hardening items **`[x]`**,
and `deploy/profiles/real/README.md` (which §0.1 itself points to via "详见") says **全部 stub/mock 填成真体 ✅**.

## Root cause — P4 vertical-cascade miss

P13-T-302 (closer) stamped the **bottom** of the doc (§4.4 🔴→🟡/🔴, §5 `[ ]`→`[x]`) but did not cascade to
the **top** boundary paragraph (§0.1). Classic "diff written ≠ cascade complete" — the same-document forward
reference was left stale, producing an internal contradiction.

## Fix

Rewrote §0.1 line 37 + line 40 to match §4.4/§5/real-README: real edition software layer fully landed
(`GetTopology` T103 · `Deploy()` T104 · real-Ascend T101 · telemetry T102 · inference T105 · authz/secret/
quota/HA/SLA T201..206 · §5 all `[x]`); only 真机运行对接验证 remains lab-gated (`tests/e2e/real/` · §4.4
五项 stamp 🔴 pending lab). Docs-only · one file.

## Verify

- stale-marker sweep (`未实现|Phase-13 stub|ErrNotImplemented|仍走 mock|待 Phase-13`) → 0 hits in §0.1
  (only the §0-intro "填成真体" framing line remains, which is correct).
- §0.1 now references P13-T-101..206 + §5 `[x]` — internally consistent with §4.4/§5 and real/README.md.
- `git status` → only `docs/build-and-production-validation.md` modified.
