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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/inference-operator/internal/metrics"
)

// NPUVerticalScalerReconciler observes NPUVerticalScaler objects and
// patches the target ModelService to switch between busy / idle
// NPUSliceTemplate refs per ADR-0012 §1 reconcile loop.
//
// Mutation model (ADR-0012 §5 adaptation):
//
// Plan §3-T007 originally specified patching
// `ModelService.spec.template.sliceTemplate`, but ModelService's existing
// schema (P4-T-103 / P5-T-007) has no such field — Phase 7 NPUSliceTemplate
// (ADR-0011 §4) is consumed by claim_controller via Pod label
// `npu.huawei.com/slice-template=<name>`, not by a ModelService spec
// field. Therefore the scaler patches the **annotation**
// `npu.huawei.com/slice-template=<name>` on ModelService.metadata.
// Phase 8 carry-forward (separate task): teach deployment_builder.go to
// read that annotation and stamp it as a Pod label so claim_controller
// (P8-T-008 wiring) sees it on PD-pair Pods. T007 alone ships the
// observe + decide + annotate loop; end-to-end Pod-label stamping is
// completed by the carry-forward task.
//
// The scaler also stamps `ocloud.edge.example.com/vertical-scaler-managed
// =<scaler-name>` per ADR-0012 §5 as the GitOps reconciliation hint
// (operators configure ArgoCD ignoreDifferences / Flux ignore for
// `.metadata.annotations.npu.huawei.com/slice-template` on annotated
// ModelServices).
type NPUVerticalScalerReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Ingestor reads windowed NPU utilization. Production: PrometheusIngestor;
	// tests: FakeIngestor.
	Ingestor metrics.Ingestor

	// Now is overridable for tests so cooldown + scaleHistory timestamps
	// are deterministic. Zero means time.Now.
	Now func() time.Time

	// ClusterScaleCap, when non-nil, returns the cluster-wide scale-event
	// usage + cap for cross-cluster rate enforcement (P13-T-204 · ADR-0025
	// §2 Decision C). A scale commit is deferred (not applied) when
	// found && cap>0 && used+1 > cap. nil → no cluster-cap enforcement
	// (backward-compatible with Phase 8 wiring). Fail-open on err.
	ClusterScaleCap func(ctx context.Context) (used, capacity int32, found bool, err error)
}

// NPUVerticalScaler-specific condition reasons.
const (
	reasonScalerReady          = "ScalerReady"
	reasonTargetNotFound       = "TargetNotFound"
	reasonScalingTransitioning = "ScalingTransitioning"
	reasonCooldownActive       = "CooldownActive"
	reasonCooldownExpired      = "CooldownExpired"
	reasonNoActiveTransition   = "NoActiveTransition"
	reasonTemplateNotFound     = "TemplateNotFound"
	reasonNoDataThisTick       = "NoDataThisTick"
	reasonClusterQuotaExceeded = "ClusterQuotaScaleRateExceeded"
)

// Pod-label / annotation keys consumed by downstream controllers.
// Mirrors ADR-0011 §4 Pod opt-in label (cluster-wide convention).
const (
	annotationSliceTemplate = "npu.huawei.com/slice-template"
)

// requeueOnNoData is the requeue cadence when Ingestor returned NoData.
// Per ADR-0012 §1 reconcile step 3.
const requeueOnNoData = 30 * time.Second

// requeueAfterScale is how long after a successful patch we wait before
// re-observing the target ModelService to capture the rolling-restart
// effect (sets ScalingInProgress=True → False once observed).
const requeueAfterScale = 60 * time.Second

func (r *NPUVerticalScalerReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Reconcile implements the controller-runtime Reconciler contract per
// ADR-0012 §1 reconcile loop:
//
//  1. Get NPUVerticalScaler → if NotFound → return nil
//  2. Get target ModelService → if NotFound → status.Active=False
//  3. Query Ingestor for window-averaged metric → if NoData → requeue 30s
//  4. Compute decision: > busy → busyTemplate · < idle → idleTemplate · else stay
//  5. If current == decision → no-op
//  6. If in cooldown → set CooldownActive=True · requeue cooldown remainder
//  7. Else: patch ModelService annotation + append ScaleEvent + lastScaleTime=now
//  8. Observe target post-patch (best-effort) → update Status.ObservedTarget
func (r *NPUVerticalScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithName("npuverticalscaler-controller").WithValues(
		"npuverticalscaler", req.NamespacedName, "task", "P8-T-007",
	)

	// Step 1: Get NPUVerticalScaler.
	var scaler inferencev1alpha1.NPUVerticalScaler
	if err := r.Client.Get(ctx, req.NamespacedName, &scaler); err != nil {
		if apierrors.IsNotFound(err) {
			lg.V(1).Info("NPUVerticalScaler deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Step 2: Get target ModelService.
	targetKey := types.NamespacedName{
		Namespace: scaler.Spec.Target.Namespace,
		Name:      scaler.Spec.Target.Name,
	}
	var target inferencev1alpha1.ModelService
	if err := r.Client.Get(ctx, targetKey, &target); err != nil {
		if apierrors.IsNotFound(err) {
			return r.markTargetNotFound(ctx, &scaler, targetKey)
		}
		return ctrl.Result{}, fmt.Errorf("get target ModelService: %w", err)
	}
	currentTemplate := target.GetAnnotations()[annotationSliceTemplate]

	// Step 3: Query Ingestor.
	//
	// P9-T-007: when metric.Type == PrometheusQuery, pass the verbatim
	// custom expression to the ingestor via CustomPromQL · skip the
	// NPUUtilization built-in PromQL shape (per ADR-0012 §7 forward note ·
	// operator owns label scoping inside the expression).
	iq := metrics.IngestorQuery{
		Namespace:     scaler.Spec.Target.Namespace,
		ModelService:  scaler.Spec.Target.Name,
		WindowSeconds: scaler.Spec.Metric.WindowSeconds,
	}
	if scaler.Spec.Metric.Type == inferencev1alpha1.MetricTypePrometheusQuery {
		iq.CustomPromQL = scaler.Spec.Metric.PrometheusQuery
	}
	result := r.Ingestor.Query(ctx, iq)
	if result.Err != nil {
		// 4xx / parse failure surfaces as Active=False reason MetricsUnreachable
		// after N persistent ticks. Phase 8 ships immediate ConditionActive
		// flip on Err (simpler than N-tick counter); refine in Phase 9 if
		// real Prometheus 5xx storms manifest noisy ConditionActive flips.
		r.Recorder.Eventf(&scaler, "Warning", "MetricsQueryError",
			"Ingestor returned error: %v", result.Err)
		if werr := r.writeStatus(ctx, &scaler, &target, currentTemplate,
			r.metricsErrCondition(result.Err)); werr != nil {
			return ctrl.Result{}, werr
		}
		return ctrl.Result{RequeueAfter: requeueOnNoData}, nil
	}
	if result.NoData {
		// Per ADR-0012 §1 reconcile step 3: stay + requeue. Do NOT flip
		// ConditionActive=False on transient NoData.
		lg.V(1).Info("Ingestor returned NoData; requeue", "after", requeueOnNoData)
		if err := r.writeStatus(ctx, &scaler, &target, currentTemplate,
			r.activeCondition(reasonNoDataThisTick)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueOnNoData}, nil
	}

	// Step 4: Compute decision.
	decision := r.decideTemplate(scaler.Spec.Metric, currentTemplate,
		scaler.Spec.ScaleSlice, result.Value)
	lg.V(1).Info("decision computed",
		"metric", result.Value,
		"busy", scaler.Spec.Metric.BusyThreshold,
		"idle", scaler.Spec.Metric.IdleThreshold,
		"current", currentTemplate,
		"target", decision.target,
		"reason", decision.reason)

	// Step 5: If current == decision → no-op.
	if decision.target == "" || decision.target == currentTemplate {
		if err := r.writeStatus(ctx, &scaler, &target, currentTemplate,
			r.activeCondition(reasonNoActiveTransition)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueOnNoData}, nil
	}

	// Step 6: Cooldown check.
	cooldownSecs := scaler.Spec.CooldownSeconds
	if cooldownSecs <= 0 {
		cooldownSecs = inferencev1alpha1.DefaultCooldownSeconds
	}
	if scaler.Status.LastScaleTime != nil {
		elapsed := r.now().Sub(scaler.Status.LastScaleTime.Time)
		cooldown := time.Duration(cooldownSecs) * time.Second
		if elapsed < cooldown {
			remainder := cooldown - elapsed
			if err := r.writeStatus(ctx, &scaler, &target, currentTemplate,
				r.cooldownActiveCondition()); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: remainder}, nil
		}
	}

	// Step 6.5 (P13-T-204): cluster-wide ClusterQuota scale-rate gate. A
	// scale event that would exceed the cluster-wide cap is deferred (not
	// committed) — defends the cluster budget even when the namespace cap
	// has headroom. Fail-open: a reader error logs + proceeds (the webhook
	// + controller tick remain the authoritative enforcement; the scaler
	// gate is best-effort defence-in-depth).
	if r.ClusterScaleCap != nil {
		used, capacity, found, err := r.ClusterScaleCap(ctx)
		if err != nil {
			lg.V(1).Info("ClusterScaleCap read failed; fail-open", "err", err.Error())
		} else if found && capacity > 0 && used+1 > capacity {
			lg.Info("scale deferred: cluster scale-rate cap reached",
				"clusterUsed", used, "clusterCap", capacity)
			r.Recorder.Eventf(&scaler, "Warning", "ClusterQuotaExceeded",
				"scale to %s deferred: cluster scale-event rate cap %d reached (current %d)",
				decision.target, capacity, used)
			if werr := r.writeStatus(ctx, &scaler, &target, currentTemplate,
				r.clusterQuotaExceededCondition(used, capacity)); werr != nil {
				return ctrl.Result{}, werr
			}
			return ctrl.Result{RequeueAfter: requeueAfterScale}, nil
		}
	}

	// Step 7: Commit scaling — patch ModelService annotation + record event.
	if err := r.patchModelServiceAnnotation(ctx, &target, &scaler, decision.target); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch target annotation: %w", err)
	}
	r.Recorder.Eventf(&scaler, "Normal", "ScalingTriggered",
		"sliceTemplate %s → %s (%s)", currentTemplate, decision.target, decision.reason)

	now := metav1.NewTime(r.now())
	scaler.Status.LastScaleTime = &now
	scaler.Status.ScaleHistory = appendScaleEvent(scaler.Status.ScaleHistory,
		inferencev1alpha1.ScaleEvent{
			Time:         now,
			FromTemplate: currentTemplate,
			ToTemplate:   decision.target,
			Reason:       decision.reason,
		})

	if err := r.writeStatus(ctx, &scaler, &target, decision.target,
		r.scalingTransitioningCondition()); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeueAfterScale}, nil
}

// decideOutcome wraps the Step-4 result.
type decideOutcome struct {
	target string
	reason string
}

// decideTemplate computes the next target template per ADR-0012 §1 step 4.
// Returns target="" when no change (stay current).
func (r *NPUVerticalScalerReconciler) decideTemplate(
	m inferencev1alpha1.MetricSpec,
	currentTemplate string,
	sl inferencev1alpha1.ScaleSliceSpec,
	value float64,
) decideOutcome {
	if value > float64(m.BusyThreshold) {
		return decideOutcome{
			target: sl.BusyTemplateName,
			reason: fmt.Sprintf("metric %.1f > busyThreshold %d", value, m.BusyThreshold),
		}
	}
	if value < float64(m.IdleThreshold) {
		return decideOutcome{
			target: sl.IdleTemplateName,
			reason: fmt.Sprintf("metric %.1f < idleThreshold %d", value, m.IdleThreshold),
		}
	}
	return decideOutcome{
		target: "",
		reason: fmt.Sprintf("metric %.1f within hysteresis [%d, %d]",
			value, m.IdleThreshold, m.BusyThreshold),
	}
}

// patchModelServiceAnnotation patches the target ModelService with the
// new sliceTemplate annotation + the management hint annotation. Uses
// MergePatchType for atomicity (a single API roundtrip, no get+update
// race window).
func (r *NPUVerticalScalerReconciler) patchModelServiceAnnotation(
	ctx context.Context,
	target *inferencev1alpha1.ModelService,
	scaler *inferencev1alpha1.NPUVerticalScaler,
	templateName string,
) error {
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				annotationSliceTemplate:               templateName,
				inferencev1alpha1.AnnotationManagedBy: scaler.Name,
			},
		},
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal annotation patch: %w", err)
	}
	return r.Client.Patch(ctx, target, client.RawPatch(types.MergePatchType, raw))
}

// appendScaleEvent enforces the rolling-window contract: max
// MaxScaleHistoryEntries entries, FIFO eviction. New entry at tail (per
// ADR-0012 §4 status.scaleHistory ordering: "oldest first").
func appendScaleEvent(history []inferencev1alpha1.ScaleEvent, ev inferencev1alpha1.ScaleEvent) []inferencev1alpha1.ScaleEvent {
	max := inferencev1alpha1.MaxScaleHistoryEntries
	history = append(history, ev)
	if len(history) > max {
		// FIFO: drop oldest.
		history = history[len(history)-max:]
	}
	return history
}

// writeStatus updates the scaler status subresource. Uses Status().Update
// to engage the status subresource handler (mirrors ModelService
// controller pattern). Updates ObservedTarget + the supplied condition.
func (r *NPUVerticalScalerReconciler) writeStatus(
	ctx context.Context,
	scaler *inferencev1alpha1.NPUVerticalScaler,
	target *inferencev1alpha1.ModelService,
	currentTemplate string,
	cond metav1.Condition,
) error {
	scaler.Status.ObservedTarget = &inferencev1alpha1.ObservedTargetStatus{
		Name:            target.Name,
		Namespace:       target.Namespace,
		CurrentTemplate: currentTemplate,
	}
	setCondition(&scaler.Status.Conditions, cond)
	return r.Client.Status().Update(ctx, scaler)
}

// markTargetNotFound writes Status.Active=False with reason TargetNotFound
// and short-circuits the reconcile pass. Does not requeue — the watch on
// ModelService objects (added in SetupWithManager) will re-trigger when
// the target is created.
func (r *NPUVerticalScalerReconciler) markTargetNotFound(
	ctx context.Context,
	scaler *inferencev1alpha1.NPUVerticalScaler,
	targetKey types.NamespacedName,
) (ctrl.Result, error) {
	r.Recorder.Eventf(scaler, "Warning", "TargetNotFound",
		"ModelService %s not found", targetKey.String())
	setCondition(&scaler.Status.Conditions, metav1.Condition{
		Type:    inferencev1alpha1.ConditionActive,
		Status:  metav1.ConditionFalse,
		Reason:  reasonTargetNotFound,
		Message: fmt.Sprintf("ModelService %s not found in namespace", targetKey.Name),
	})
	if err := r.Client.Status().Update(ctx, scaler); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{}, nil
}

// activeCondition builds a generic ConditionActive=True with the supplied
// reason (used for ScalerReady / NoActiveTransition / NoDataThisTick).
func (r *NPUVerticalScalerReconciler) activeCondition(reason string) metav1.Condition {
	return metav1.Condition{
		Type:   inferencev1alpha1.ConditionActive,
		Status: metav1.ConditionTrue,
		Reason: reason,
	}
}

func (r *NPUVerticalScalerReconciler) cooldownActiveCondition() metav1.Condition {
	return metav1.Condition{
		Type:    inferencev1alpha1.ConditionCooldownActive,
		Status:  metav1.ConditionTrue,
		Reason:  reasonCooldownActive,
		Message: "Cooldown window active; scaling decision deferred",
	}
}

func (r *NPUVerticalScalerReconciler) scalingTransitioningCondition() metav1.Condition {
	return metav1.Condition{
		Type:    inferencev1alpha1.ConditionScalingInProgress,
		Status:  metav1.ConditionTrue,
		Reason:  reasonScalingTransitioning,
		Message: "Scaling transition committed; waiting for rolling-restart effect",
	}
}

// clusterQuotaExceededCondition reports a scale deferred by the cluster-wide
// ClusterQuota scale-rate cap (P13-T-204). Active stays True (the scaler is
// healthy — it is intentionally holding back), surfaced as a distinct
// condition type so operators can alert on cluster-budget pressure.
func (r *NPUVerticalScalerReconciler) clusterQuotaExceededCondition(used, capacity int32) metav1.Condition {
	return metav1.Condition{
		Type:    inferencev1alpha1.ConditionActive,
		Status:  metav1.ConditionTrue,
		Reason:  reasonClusterQuotaExceeded,
		Message: fmt.Sprintf("scale deferred: cluster scale-event rate cap %d reached (current %d)", capacity, used),
	}
}

func (r *NPUVerticalScalerReconciler) metricsErrCondition(err error) metav1.Condition {
	return metav1.Condition{
		Type:    inferencev1alpha1.ConditionActive,
		Status:  metav1.ConditionFalse,
		Reason:  "MetricsUnreachable",
		Message: fmt.Sprintf("Ingestor error: %v", err),
	}
}

// setCondition upserts a condition by type into the supplied slice.
// Mirrors metav1's SetStatusCondition semantics without importing the
// helper (avoids adding a new dep edge through apimachinery/pkg/util/
// random tag).
func setCondition(conds *[]metav1.Condition, c metav1.Condition) {
	if c.LastTransitionTime.IsZero() {
		c.LastTransitionTime = metav1.Now()
	}
	for i := range *conds {
		if (*conds)[i].Type == c.Type {
			if (*conds)[i].Status != c.Status {
				c.LastTransitionTime = metav1.Now()
			} else {
				c.LastTransitionTime = (*conds)[i].LastTransitionTime
			}
			(*conds)[i] = c
			return
		}
	}
	*conds = append(*conds, c)
}

// SetupWithManager registers this Reconciler with the manager. Watches
// inference.ocloud.edge.example.com/v1alpha1.NPUVerticalScaler.
func (r *NPUVerticalScalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&inferencev1alpha1.NPUVerticalScaler{}).
		Named("npuverticalscaler").
		Complete(r)
}
