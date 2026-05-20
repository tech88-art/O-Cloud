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
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/inference-operator/internal/metrics"
)

// FinalizerName is the metadata.finalizers entry the ModelService
// controller stamps onto every observed ModelService. The finalizer
// blocks deletion until the controller has drained owned resources
// (Deployments + ResourceClaims created in T007/T008).
const FinalizerName = "inference.ocloud.edge.example.com/modelservice-cleanup"

// Condition types written to ModelService.Status.Conditions.
const (
	// ConditionPoolUnresolved is False with reason NPUSlicePoolNotFound
	// when the named NPUSlicePool does not exist in the same namespace
	// as the ModelService.
	ConditionPoolUnresolved = "PoolUnresolved"

	// ConditionAllocationReady is True once every per-replica
	// ResourceClaim created by T007 reports an AllocatedDeviceStatus
	// with Ready=True. Surface signal for ModelService consumers.
	ConditionAllocationReady = "AllocationReady"

	// ConditionAvailable is True when phase=Ready (PD pair healthy).
	ConditionAvailable = "Available"
)

// Reasons used on the conditions above.
const (
	reasonNPUSlicePoolNotFound = "NPUSlicePoolNotFound"
	reasonNPUSlicePoolFound    = "NPUSlicePoolFound"
	reasonWaitingForPool       = "WaitingForPool"
)

// poolGVK is the GroupVersionKind the controller uses for unstructured
// reads against the pool-operator's NPUSlicePool CRD. Cross-module Go
// imports are forbidden by operators/CLAUDE.md §1 — the unstructured
// path keeps the build dependency edge clean.
var poolGVK = schema.GroupVersionKind{
	Group:   "ims.ocloud.edge.example.com",
	Version: "v1alpha1",
	Kind:    "NPUSlicePool",
}

// ModelServiceReconciler observes
// inference.ocloud.edge.example.com/v1alpha1.ModelService objects and
// orchestrates the Phase 5 provisioning flow: resolve the bound
// NPUSlicePool, create per-replica ResourceClaims (T007), spawn the
// Prefill + Decode Deployments (T007), drive Status.Phase through
// Pending → Provisioning → Ready / Failed (T008).
//
// T006 (this commit) ships the minimal scaffold:
//   - finalizer add path (deletion-drain path arrives T008)
//   - NPUSlicePool resolution via unstructured client
//   - phase transition: Pending → Provisioning (when pool found and at
//     least some slices observed) or Failed (when pool not found)
//   - WaitingForPool event when pool exists but observes 0 slices
type ModelServiceReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Now is overridable for tests so the ProgressDeadline check is
	// exercisable without manipulating system time. Zero means
	// time.Now.
	Now func() time.Time
}

func (r *ModelServiceReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Reconcile implements the controller-runtime Reconciler contract.
func (r *ModelServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// P6-T-104: observe Reconcile duration. defer pattern captures all
	// return paths (NotFound short-circuit + happy path + finalizer +
	// phase write + error).
	start := r.now()
	defer func() {
		metrics.ObserveReconcileDuration(r.now().Sub(start).Seconds())
	}()

	lg := log.FromContext(ctx).WithName("modelservice-controller").WithValues(
		"modelservice", req.NamespacedName, "task", "P5-T-006",
	)

	var ms inferencev1alpha1.ModelService
	if err := r.Client.Get(ctx, req.NamespacedName, &ms); err != nil {
		if apierrors.IsNotFound(err) {
			lg.V(1).Info("ModelService deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if ms.DeletionTimestamp != nil {
		// Phase 5 T006: deletion-drain logic is a Phase 5 T008
		// deliverable. T006 only removes the finalizer when no owned
		// resources exist (a no-op since T006 doesn't create any).
		// T007/T008 expand this to wait for Deployments + ResourceClaims
		// to be GC'd before unblocking deletion.
		controllerutil.RemoveFinalizer(&ms, FinalizerName)
		if err := r.Client.Update(ctx, &ms); err != nil {
			if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&ms, FinalizerName) {
		controllerutil.AddFinalizer(&ms, FinalizerName)
		if err := r.Client.Update(ctx, &ms); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		// Requeue immediately so the new ResourceVersion drives the
		// rest of the reconcile pass without the stale finalizer
		// state interfering.
		return ctrl.Result{Requeue: true}, nil
	}

	pool, poolErr := r.resolveNPUSlicePool(ctx, ms.Namespace, ms.Spec.NPUSlicePoolRef.Name)
	switch {
	case apierrors.IsNotFound(poolErr):
		return r.markPoolNotFound(ctx, &ms, ms.Spec.NPUSlicePoolRef.Name)
	case poolErr != nil:
		return ctrl.Result{}, fmt.Errorf("resolve NPUSlicePool: %w", poolErr)
	}

	if !poolHasSlices(pool) {
		return r.markWaitingForPool(ctx, &ms, pool)
	}

	if err := r.reconcileChildren(ctx, &ms); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile children: %w", err)
	}

	return r.reconcilePhase(ctx, &ms, pool)
}

// reconcilePhase aggregates child state via the phase machine
// (phases.go) and writes the resulting Phase + Conditions to the
// ModelService status subresource.
func (r *ModelServiceReconciler) reconcilePhase(ctx context.Context, ms *inferencev1alpha1.ModelService, pool *unstructured.Unstructured) (ctrl.Result, error) {
	in, err := r.assemblePhaseInputs(ctx, ms)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("assemble phase inputs: %w", err)
	}
	decision := computePhase(in, &ms.Status)

	base := ms.DeepCopy()
	ms.Status.Phase = decision.Phase
	ms.Status.ObservedGeneration = ms.Generation

	// P6-T-104: record phase transition when Status.Phase actually
	// changes. RecordPhaseTransition treats the recompute-same-phase
	// case as a no-op via the `from != to` guard here.
	if string(base.Status.Phase) != string(decision.Phase) {
		metrics.RecordPhaseTransition(string(base.Status.Phase), string(decision.Phase))
	}

	// PoolUnresolved stays True from the pool-found check we already
	// did above.
	SetCondition(&ms.Status.Conditions, metav1.Condition{
		Type:    ConditionPoolUnresolved,
		Status:  metav1.ConditionTrue,
		Reason:  reasonNPUSlicePoolFound,
		Message: fmt.Sprintf("NPUSlicePool %s/%s observed", ms.Namespace, pool.GetName()),
	})
	for _, c := range decision.Conditions {
		SetCondition(&ms.Status.Conditions, c)
	}
	if err := r.Client.Status().Patch(ctx, ms, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch phase status: %w", err)
	}

	if r.Recorder != nil && decision.Phase == inferencev1alpha1.PhaseReady && base.Status.Phase != inferencev1alpha1.PhaseReady {
		r.Recorder.Event(ms, corev1.EventTypeNormal, string(inferencev1alpha1.PhaseReady),
			"ModelService is Ready (Prefill+Decode pair healthy)")
	}
	if r.Recorder != nil && decision.Phase == inferencev1alpha1.PhaseFailed && base.Status.Phase != inferencev1alpha1.PhaseFailed {
		r.Recorder.Event(ms, corev1.EventTypeWarning, string(inferencev1alpha1.PhaseFailed),
			"ModelService transitioned to Failed; inspect conditions for details")
	}

	// Requeue while still Provisioning so we re-check readiness on
	// the next tick even without an external watch event.
	if decision.Phase == inferencev1alpha1.PhaseProvisioning {
		return RequeueAfter(30 * time.Second), nil
	}
	return ctrl.Result{}, nil
}

// assemblePhaseInputs gathers Deployment + ResourceClaim state for
// this ModelService and returns a PhaseInputs snapshot.
func (r *ModelServiceReconciler) assemblePhaseInputs(ctx context.Context, ms *inferencev1alpha1.ModelService) (PhaseInputs, error) {
	in := PhaseInputs{
		PrefillDesired: ms.Spec.PDPair.Prefill.Replicas,
		DecodeDesired:  ms.Spec.PDPair.Decode.Replicas,
		Now:            r.now(),
	}

	var prefill, decode appsv1.Deployment
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ms.Namespace, Name: deploymentName(ms, PDSidePrefill)}, &prefill); err == nil {
		in.PrefillReady = prefill.Status.ReadyReplicas
		in.PrefillProgressing = progressingCondition(&prefill)
	} else if !apierrors.IsNotFound(err) {
		return in, err
	}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ms.Namespace, Name: deploymentName(ms, PDSideDecode)}, &decode); err == nil {
		in.DecodeReady = decode.Status.ReadyReplicas
		in.DecodeProgressing = progressingCondition(&decode)
	} else if !apierrors.IsNotFound(err) {
		return in, err
	}

	var claims resourceapi.ResourceClaimList
	if err := r.Client.List(ctx, &claims, client.InNamespace(ms.Namespace)); err != nil {
		return in, err
	}
	in.ClaimsTotal, in.ClaimsAllocated = countClaimsAllocated(ms, claims.Items)
	return in, nil
}

// reconcileChildren creates or updates the Prefill + Decode
// Deployments and the per-side ResourceClaimTemplate. Idempotent.
// Phase 5 T007: create/update only; T008 adds owner-ref-based GC
// for stale templates / Deployments left behind by replica or image
// changes.
func (r *ModelServiceReconciler) reconcileChildren(ctx context.Context, ms *inferencev1alpha1.ModelService) error {
	for _, side := range []PDSide{PDSidePrefill, PDSideDecode} {
		if err := r.reconcileResourceClaimTemplate(ctx, ms, side); err != nil {
			return err
		}
		if err := r.reconcileDeployment(ctx, ms, side); err != nil {
			return err
		}
	}
	return nil
}

func (r *ModelServiceReconciler) reconcileResourceClaimTemplate(ctx context.Context, ms *inferencev1alpha1.ModelService, side PDSide) error {
	desired := buildResourceClaimTemplate(ms, side)
	if err := controllerutil.SetControllerReference(ms, desired, r.Scheme); err != nil {
		return fmt.Errorf("set controller ref on ResourceClaimTemplate %s/%s: %w", desired.Namespace, desired.Name, err)
	}

	var existing resourceapi.ResourceClaimTemplate
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Client.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	// ResourceClaimTemplate.Spec is immutable per upstream contract.
	// Phase 5 T007 leaves Spec drift alone; T008 may delete + recreate
	// on a model-spec change. For now, refresh only labels/annotations
	// at the top-level metadata (Spec stays as the existing template).
	if !reflect.DeepEqual(existing.Labels, desired.Labels) {
		existing.Labels = desired.Labels
		return r.Client.Update(ctx, &existing)
	}
	return nil
}

func (r *ModelServiceReconciler) reconcileDeployment(ctx context.Context, ms *inferencev1alpha1.ModelService, side PDSide) error {
	desired := buildDeployment(ms, side)
	if err := controllerutil.SetControllerReference(ms, desired, r.Scheme); err != nil {
		return fmt.Errorf("set controller ref on Deployment %s/%s: %w", desired.Namespace, desired.Name, err)
	}

	var existing appsv1.Deployment
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Client.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Replica / image / args drift → update in place. The Pod template
	// + selectors stay otherwise unchanged so a rolling update fires
	// only when needed.
	changed := false
	if !ptrInt32Equal(existing.Spec.Replicas, desired.Spec.Replicas) {
		existing.Spec.Replicas = desired.Spec.Replicas
		changed = true
	}
	if !reflect.DeepEqual(existing.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers) {
		existing.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
		changed = true
	}
	if !reflect.DeepEqual(existing.Spec.Template.Spec.ResourceClaims, desired.Spec.Template.Spec.ResourceClaims) {
		existing.Spec.Template.Spec.ResourceClaims = desired.Spec.Template.Spec.ResourceClaims
		changed = true
	}
	if changed {
		return r.Client.Update(ctx, &existing)
	}
	return nil
}

func ptrInt32Equal(a, b *int32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// resolveNPUSlicePool reads the named pool via the unstructured client.
// Returns NotFound when the pool does not exist in the same namespace
// as the ModelService.
func (r *ModelServiceReconciler) resolveNPUSlicePool(ctx context.Context, ns, name string) (*unstructured.Unstructured, error) {
	pool := &unstructured.Unstructured{}
	pool.SetGroupVersionKind(poolGVK)
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, pool); err != nil {
		return nil, err
	}
	return pool, nil
}

// poolHasSlices returns true when the pool's status.totalSlices field
// is > 0 — Phase 5 readiness heuristic. T007 may refine to also
// require AvailableSlices > requested replica count.
func poolHasSlices(pool *unstructured.Unstructured) bool {
	totalI, found, err := unstructured.NestedInt64(pool.Object, "status", "totalSlices")
	if err != nil || !found {
		return false
	}
	return totalI > 0
}

func (r *ModelServiceReconciler) markPoolNotFound(ctx context.Context, ms *inferencev1alpha1.ModelService, poolName string) (ctrl.Result, error) {
	base := ms.DeepCopy()
	ms.Status.Phase = inferencev1alpha1.PhaseFailed
	ms.Status.ObservedGeneration = ms.Generation
	SetCondition(&ms.Status.Conditions, metav1.Condition{
		Type:    ConditionPoolUnresolved,
		Status:  metav1.ConditionFalse,
		Reason:  reasonNPUSlicePoolNotFound,
		Message: fmt.Sprintf("NPUSlicePool %s/%s not found", ms.Namespace, poolName),
	})
	if err := r.Client.Status().Patch(ctx, ms, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch failed status: %w", err)
	}
	if r.Recorder != nil {
		r.Recorder.Eventf(ms, corev1.EventTypeWarning, reasonNPUSlicePoolNotFound,
			"NPUSlicePool %s/%s not found", ms.Namespace, poolName)
	}
	return ctrl.Result{}, nil
}

func (r *ModelServiceReconciler) markWaitingForPool(ctx context.Context, ms *inferencev1alpha1.ModelService, pool *unstructured.Unstructured) (ctrl.Result, error) {
	base := ms.DeepCopy()
	ms.Status.Phase = inferencev1alpha1.PhaseProvisioning
	ms.Status.ObservedGeneration = ms.Generation
	SetCondition(&ms.Status.Conditions, metav1.Condition{
		Type:    ConditionPoolUnresolved,
		Status:  metav1.ConditionTrue,
		Reason:  reasonNPUSlicePoolFound,
		Message: fmt.Sprintf("NPUSlicePool %s/%s observed; waiting for slices", ms.Namespace, pool.GetName()),
	})
	SetCondition(&ms.Status.Conditions, metav1.Condition{
		Type:    ConditionAllocationReady,
		Status:  metav1.ConditionFalse,
		Reason:  reasonWaitingForPool,
		Message: "Pool has no slices yet; provisioning blocked",
	})
	if err := r.Client.Status().Patch(ctx, ms, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch waiting-for-pool status: %w", err)
	}
	if r.Recorder != nil {
		r.Recorder.Eventf(ms, corev1.EventTypeNormal, reasonWaitingForPool,
			"NPUSlicePool %s/%s observed but reports 0 slices yet", ms.Namespace, pool.GetName())
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers this Reconciler with the manager. Watches
// inference.ocloud.edge.example.com/v1alpha1.ModelService.
func (r *ModelServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&inferencev1alpha1.ModelService{}).
		Named("modelservice").
		Complete(r)
}
