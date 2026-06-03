// Standalone P99 SLA load-test harness (P13-T-206 · ADR-0025 §2 Decision E).
// Its OWN module — stdlib-only, no external deps — so it builds/runs anywhere
// (incl. arm64 lab nodes) without touching the operator module graphs. NOT in
// the CI Go matrix (it is a lab-driven tool); correctness is covered by its own
// percentile_test.go run locally.
module ocloud.edge.example.com/sla-harness

go 1.22
