# caveman

**Addresses failure mode**: Code entropy — token economy specifically. See `~/.claude/CODING.md` § 4.

Token-frugal communication. For long O-Cloud project sessions where context fills fast (multi-module + shared contract + agent coordination + ADR history).

## When to use

After ~30 minutes of session, when context budget feels tight, or when the user invokes `/caveman`.

## Process

Drop:
- Articles: a / an / the.
- Filler: just / really / basically / actually / simply.
- Hedging: I think / perhaps / it seems.
- Pleasantries: thanks / sure / of course.

Keep:
- Exact technical terminology — `NPUSlicePool`, `ResourceClaim`, `client-go`, `Kubebuilder`, `Karmada`, `DRA`, `MindIE`, `vLLM` — **do not abbreviate further**.
- Numbers and units (`65536 MiB`, `30 fps`, `200ms p99`).
- File paths and line numbers (`backend/pkg/datasource/source.go:88`).
- Task package IDs (`P1-T-101`, `P1-T-108b`).

Pattern: `[thing] [action] [reason]. [next step].`

## Example

**Before** (107 chars): "I think we should probably just basically refactor the topology handler to also aggregate from the CRD source."

**After** (52 chars): "Refactor topology handler. Add CRD source aggregation."

## Anti-patterns

- Abbreviating technical terms (e.g. "k8s ds" for "DaemonSet", "fe" for "frontend") — saves tokens, creates ambiguity. Forbidden.
- Dropping units. "5 ms" stays. "5" loses information.
- **Never** further-abbreviate K8s / Ascend acronyms — DRA / CRD / NPU / HCCS / TTI are already abbreviations; mangling them violates project terminology contract.
- Compressing task package IDs (`P1-T-108b` → "T108b" inline is OK; "108" alone is not).

## Why this skill exists

O-Cloud sessions accumulate context across architecture / module CLAUDE / agent coordination / shared contracts / mock data / ADRs. Token entropy grows non-linearly. Cutting filler keeps technical detail in the working window.
