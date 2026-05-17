# 0000 — Record architecture decisions

## Status

Accepted

## Context

We need to record the architectural decisions made on this project, so that future contributors (human and Claude) can understand *why* the code is the way it is — not just *what* it does. Without this, every refactor relitigates settled questions, and every new contributor re-derives reasoning that should already be cached.

This project's surface (RAN edge AI inference) accumulates decisions quickly: hardware backends, quantization schemes, numerical tolerances, operator coverage, language splits. Each one is hard to reverse.

## Decision

We will use ADRs as described by Michael Nygard:
https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions

Each ADR lives in `docs/adr/NNNN-short-title.md` with sections: **Status**, **Context**, **Decision**, **Consequences**.

Write an ADR when:

1. Adding a new HW backend (changes hardware abstraction surface).
2. Adding a new quantization scheme (changes numerical contracts).
3. Dropping or breaking a public operator (changes external surface).

ADRs are accepted by merging the PR that adds them. Superseded ADRs are not deleted; their Status changes to "Superseded by ADR-XXXX".

## Consequences

See Michael Nygard's article, linked above.
