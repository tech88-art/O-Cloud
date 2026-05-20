/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package metrics exposes Prometheus collectors for the inference-operator
// per Phase 6 P6-T-104. Three collectors registered against the
// controller-runtime global metrics registry (served on the manager's
// configured `--metrics-bind-address`, default :8082):
//
//   - inference_modelservice_phase_transitions_total{from,to}     Counter
//   - inference_pdrouter_decisions_total{decision}                Counter
//   - inference_modelservice_reconcile_duration_seconds            Histogram
//
// Plan §4 P6-T-104 originally listed 4 collectors including
// "allocator picks", but that counter was dropped at task entry because
// inference-operator does NOT call npu-dra-driver's allocator directly —
// allocation happens via DRA's ResourceClaim machinery. Allocator metrics
// belong in npu-dra-driver (Phase 7+ candidate).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Webhook decision label values for inference_pdrouter_decisions_total.
const (
	WebhookDecisionAllowedNoPatch = "allowed_no_patch"
	WebhookDecisionPatched        = "patched"
	WebhookDecisionDenied         = "denied"
)

var (
	// PhaseTransitions counts ModelService phase changes by (from, to).
	// Incremented from the ModelServiceReconciler when Status.Phase
	// actually changes (not on each reconcile that recomputes the same
	// phase). Empty `from` indicates the initial transition from no
	// prior phase to the first computed phase.
	PhaseTransitions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "inference_modelservice_phase_transitions_total",
			Help: "Total number of ModelService phase transitions, labelled by from and to.",
		},
		[]string{"from", "to"},
	)

	// WebhookDecisions counts PD Router admission decisions.
	// decision ∈ {allowed_no_patch, patched, denied}.
	WebhookDecisions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "inference_pdrouter_decisions_total",
			Help: "Total number of PD Router mutating admission webhook decisions, labelled by outcome.",
		},
		[]string{"decision"},
	)

	// ReconcileDuration is a histogram of ModelService reconcile durations
	// in seconds. Buckets cover the expected Phase 6 simulator-scope range
	// (< 10ms fast path; up to ~10s for pool resolution + child
	// reconciliation). Histogram instead of Summary so multiple replicas
	// can be aggregated at scrape time via Prometheus's _sum/_count math.
	ReconcileDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "inference_modelservice_reconcile_duration_seconds",
			Help:    "ModelService Reconcile loop wall-clock duration in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10},
		},
	)
)

func init() {
	// Register against controller-runtime's shared registry. The manager's
	// metrics server (configured via ctrl.Options.Metrics) serves this
	// registry at /metrics.
	ctrlmetrics.Registry.MustRegister(
		PhaseTransitions,
		WebhookDecisions,
		ReconcileDuration,
	)
}

// RecordPhaseTransition increments PhaseTransitions with the given labels.
// Callers must guarantee `from != to` (recomputing the same phase is NOT
// a transition); the function does not enforce this so it stays cheap
// in the hot path.
func RecordPhaseTransition(from, to string) {
	PhaseTransitions.WithLabelValues(from, to).Inc()
}

// RecordWebhookDecision increments WebhookDecisions with the given
// decision label value. Use the WebhookDecision* constants above.
func RecordWebhookDecision(decision string) {
	WebhookDecisions.WithLabelValues(decision).Inc()
}

// ObserveReconcileDuration records a single reconcile duration sample
// to ReconcileDuration. Typical usage:
//
//	defer metrics.ObserveReconcileDuration(time.Since(start))
func ObserveReconcileDuration(seconds float64) {
	ReconcileDuration.Observe(seconds)
}
