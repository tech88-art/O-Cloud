# task-package

**Addresses failure mode**: Misalignment + Code entropy. See `~/.claude/CODING.md` §§ 1, 4.

Execution discipline for an O-Cloud Phase 1 task package (`docs/tasks/P1-T-XXX.md`). The task package is a **contract**; this skill ensures it's honored.

## When to use

Whenever a new task package is assigned (by user in chat or by main agent dispatching to a subagent). Fire on every task — not optional.

## Process

### 1. Startup checklist (every task, before any edit)

```
[ ] Read root CLAUDE.md
[ ] Read <module>/CLAUDE.md (your bound module)
[ ] Read docs/agent-coordination.md §0a (operative single-agent rules) + §1-§12 reference
[ ] Read the task package docs/tasks/P1-T-XXX.md fully
[ ] git fetch && git status — confirm clean working tree
[ ] Verify Depends on tasks are all merged to dev (git log dev --grep "P1-T-YYY")
[ ] git checkout -b <type>/p1-t-<id>-<short-desc> from dev
```

Skip any step → STOP. Do not start coding.

### 2. Allowed Paths enforcement (before EVERY edit)

Before each `Edit` / `Write` call, verify:

- The file is in the task package's **Allowed Paths** white-list
- The file is NOT in **Forbidden Paths**
- The file is NOT a **shared contract** (`docs/api-contract.yaml`, `configs/mock-data/schema.json`, `operators/pool-operator/api/v1alpha1/*.go`) unless this task explicitly owns it

If any check fails → STOP, do not edit. Propose change via chat (per agent-coordination §0a.5) — user approves, ADR captures the decision, then proceed.

### 3. Acceptance Criteria → machine-verifiable evidence

Each AC must produce a concrete evidence artifact in the PR:
- "test passes" → paste exact `go test` / `pnpm test` output
- "lint clean" → paste `golangci-lint` / `eslint` output
- "contract aligned" → cite the CI job result
- "user approval" → paste chat link or quote

If an AC is fuzzy ("looks good", "协调者签字") → flag in chat for hardening before coding (see `grill-me` skill).

### 4. PR description (per agent-coordination §5.3)

Mandatory fields:
```markdown
## Task
P1-T-XXX

## Summary
<one sentence>

## Changes
- <bullet>

## Allowed Paths Check
- [x] Modified files all within task's Allowed Paths

## Acceptance Criteria
- [x] <criterion 1> — <evidence>
- [x] <criterion 2> — <evidence>

## Testing
<paste command output>

## Related
- Depends on: <list>
- Blocks: <list>
```

Conventional Commits title: `<type>(<scope>): <subject>` (e.g., `feat(backend): implement cluster topology API`).

### 5. Commit & merge

- Squash merge target: `dev` (never main)
- Required: CI all green + user approval in chat
- Forbidden: `git push --force`, `git commit --no-verify`, direct push to dev/main

### 6. Subagent fan-out decision (if main agent)

Before deciding to spawn a subagent for parts of this task, verify (per agent-coordination §0a.2):

- [ ] Sub-parts are **independent** (no shared branch conflicts)
- [ ] Sub-parts' Allowed Paths **do not overlap**
- [ ] Sub-parts each complete within one subagent session
- [ ] Parallelism saves ≥ 30% wall-clock (otherwise sequential)

If any check fails → execute sequentially in main agent.

## Anti-patterns

- Editing outside Allowed Paths because "it's a quick fix" (never — go through ADR).
- Skipping the startup checklist because "it's a small task" (no exceptions — see agent-coordination §9).
- Bundling multiple task packages in one PR (one task = one PR; if scope grows, split).
- Vague PR description ("update some files") — must follow §5.3 template fully.
- Subagent modifying a shared contract (never — main agent串行).
- Force-pushing to a branch after PR review started.

## Why this skill exists

Multi-module projects with strict path ownership fail when a single agent loses track of which file belongs to which module / which task. The task package is a small contract; honoring it prevents accidental cross-module damage, contract drift, and PR rejection. The skill makes the discipline explicit and check-driven, not memory-driven.
