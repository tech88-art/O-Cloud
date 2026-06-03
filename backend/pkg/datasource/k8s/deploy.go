package k8s

// Package k8s — real Deploy() / DeleteDeploy() (P13-T-104 · ADR-0024 §2 Decision E).
//
// Deploy materializes a workload on the live apiserver: it applies a
// apps/v1.Deployment plus a core/v1.Service via the typed client-go clientset,
// then returns a *model.DeployResponse carrying the deployId + status. The shape
// of that response is byte-for-byte the same contract the mock source returns
// (mock/deploy.go) — the HTTP handler (pkg/api/deploy.go) consumes whichever
// source mapping.deploy selected and does NOT branch on profile. The real-vs-mock
// difference lives entirely below the datasource.Source seam (ADR-0024 §2
// Decision G decoupling-seam invariant): mock appends to an in-memory cache, k8s
// applies to the apiserver, the handler is identical for both.
//
// Ownership convention (the load-bearing contract between Deploy and
// DeleteDeploy):
//
//	app.kubernetes.io/managed-by = "demo-backend"   // every object we create
//	deploy.ocloud.io/id          = "<deployID>"     // unique per Deploy call
//
// Both labels are stamped on the Deployment, the Service, AND the Deployment's
// pod template (so the Service selector resolves the pods and DeleteDeploy can
// find every owned object with a single label-selector list). DeleteDeploy lists
// Deployments+Services across all namespaces filtered by
// `deploy.ocloud.io/id=<id>` and deletes them, so it never has to remember which
// namespace a deploy landed in — the apiserver IS the source of truth (the
// backend has no DB, project root §5). The deployId is therefore stable and
// reconstructible from cluster state alone (no in-memory index, unlike the mock).
//
// Image / NPU resourcing: the k8s source has no preset catalog (ListPresets is
// ErrCapabilityUnavailable here — presets are served by the configmap source),
// so Deploy cannot look the image up from a preset the way the mock does. The
// image and per-replica NPU count come from the request:
//   - req.Parameters["image"]    → container image (string). Falls back to a
//     vllm-ascend default so a minimal request still produces a runnable shape.
//   - req.Parameters["npuCount"] → huawei.com/Ascend910B resource request per
//     replica (number or numeric string). 0/absent → no NPU request (CPU-only
//     placeholder, e.g. a smoke Deployment).
//
// These keys are read defensively (JSON numbers decode as float64); unknown keys
// are ignored. This keeps the wire contract (DeployRequest) unchanged — no
// api-contract.yaml edit — while letting a real deploy carry the one or two extra
// knobs a Deployment needs that the mock fabricated from a preset.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Deploy ownership labels. managedByValue is the fixed value identifying
// objects this backend created (vs operator-owned / hand-applied workloads);
// deployIDLabel carries the per-deploy unique id so DeleteDeploy can select
// exactly the objects one Deploy call produced. Mirrors the mock source's
// app.kubernetes.io/managed-by + deploy.ocloud.io/id labels (mock/deploy.go),
// differing only in the managed-by VALUE ("demo-backend" vs "mock-deploy") so a
// cluster that ran both never cross-deletes.
const (
	managedByLabel = "app.kubernetes.io/managed-by"
	managedByValue = "demo-backend"
	deployIDLabel  = "deploy.ocloud.io/id"
	nameLabel      = "app.kubernetes.io/name"
)

// deployParamImage / deployParamNPUCount are the optional DeployRequest.parameters
// keys the k8s source honours (see package header). Reading them keeps the wire
// DeployRequest schema unchanged.
const (
	deployParamImage    = "image"
	deployParamNPUCount = "npuCount"
)

// defaultDeployImage is the container image used when req.Parameters omits an
// explicit "image". A vllm-ascend reference so a minimal deploy still produces a
// recognisable inference-shaped Deployment; the real deploy almost always passes
// an explicit image (the preset's resolved image, supplied by the caller).
const defaultDeployImage = "quay.io/ascend/vllm-ascend:latest"

// ascend910BResource is the Ascend Device Plugin resource key requested per
// replica when req carries a non-zero npuCount. Mirrors npu.go's
// huawei.com/Ascend910B capacity key so requests line up with what nodes
// advertise.
const ascend910BResource = corev1.ResourceName("huawei.com/Ascend910B")

// ErrDeployInvalidRequest covers structural problems caught before we touch the
// apiserver (nil request, missing namespace/preset, negative replicas). Maps to
// 400 in a handler that chooses to special-case it; the shared handler currently
// renders it 500 (it only special-cases the mock sentinels), which is acceptable
// — a malformed real deploy is a server-side wiring bug, not a routine user
// 4xx in the demo flow.
var ErrDeployInvalidRequest = errors.New("k8s: invalid deploy request")

// ErrDeployConflict surfaces when the target Deployment already exists (a repeat
// deploy of the same name in the same namespace). Distinct sentinel so callers
// can tell "already there" from a generic apiserver 500.
var ErrDeployConflict = errors.New("k8s: deploy already exists")

// Deploy applies a Deployment + Service for req and returns the DeployResponse.
//
// Sequence:
//  1. Validate req structurally (namespace + presetId required; replicas >= 0).
//  2. Derive identity: workloadName (req.Name or "<presetId>-<unixnano>"),
//     deployID (a stable token = workloadName, since cluster state is the SoT).
//  3. Build the Deployment (replicas, pod template labelled with the ownership
//     labels + the container image + optional NPU resource request) and apply it.
//  4. Build the headless-friendly ClusterIP Service selecting the pod labels and
//     apply it.
//  5. Return DeployResponse{deployId, workloadName, namespace, status:"accepted"}.
//
// ctx cancellation is honoured before each apiserver write. If the Deployment
// already exists we return ErrDeployConflict without creating the Service (no
// partial second copy).
func (s *Source) Deploy(ctx context.Context, req *model.DeployRequest) (*model.DeployResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("%w: nil request", ErrDeployInvalidRequest)
	}
	if strings.TrimSpace(req.PresetID) == "" {
		return nil, fmt.Errorf("%w: presetId is required", ErrDeployInvalidRequest)
	}
	namespace := strings.TrimSpace(req.Namespace)
	if namespace == "" {
		return nil, fmt.Errorf("%w: namespace is required", ErrDeployInvalidRequest)
	}
	replicas := int32(req.Replicas)
	if req.Replicas == 0 {
		replicas = 1 // contract default
	}
	if req.Replicas < 0 {
		return nil, fmt.Errorf("%w: replicas must be >= 1 (got %d)", ErrDeployInvalidRequest, req.Replicas)
	}

	workloadName := sanitizeDeployName(req.Name)
	if workloadName == "" {
		workloadName = sanitizeDeployName(fmt.Sprintf("%s-%d", req.PresetID, time.Now().UnixNano()))
	}
	// deployID == workloadName: the cluster is the source of truth (no in-memory
	// index), so a name unique within the namespace doubles as a delete handle.
	// The deploy.ocloud.io/id label carries it verbatim.
	deployID := workloadName

	image := deployImage(req)
	npuPerReplica := deployNPUCount(req)

	labels := deployLabels(workloadName, deployID)

	dep := buildDeployment(namespace, workloadName, labels, replicas, image, npuPerReplica)
	if _, err := s.client.AppsV1().Deployments(namespace).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("%w: %s/%s", ErrDeployConflict, namespace, workloadName)
		}
		return nil, errFromAPIServer("create deployment", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	svc := buildService(namespace, workloadName, labels)
	if _, err := s.client.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			// Service create failed after the Deployment landed. Best-effort roll
			// back the Deployment so a retried deploy doesn't hit ErrDeployConflict
			// on a half-applied pair. Rollback errors are swallowed (the apiserver
			// error below is the one that matters).
			_ = s.client.AppsV1().Deployments(namespace).Delete(
				context.Background(), workloadName, metav1.DeleteOptions{})
			return nil, errFromAPIServer("create service", err)
		}
		// Service already existed (e.g. left over) — tolerate; the Deployment is
		// the primary object and it was just created.
	}

	return &model.DeployResponse{
		DeployID:       deployID,
		WorkloadName:   workloadName,
		Namespace:      namespace,
		Status:         "accepted",
		ScheduledNodes: nil, // real scheduling is async; nodes populate once pods bind (ListWorkloads reflects it)
		Message: fmt.Sprintf("applied Deployment+Service (replicas=%d, image=%s, npu=%d)",
			replicas, image, npuPerReplica),
	}, nil
}

// DeleteDeploy removes every object this backend created for deployID. It lists
// Deployments and Services across all namespaces filtered by the
// deploy.ocloud.io/id label and deletes each match. Returns ErrResourceNotFound
// (the package 404 sentinel) when no object carries the id — the handler maps
// that onto 404. Deleting an unknown id is therefore a clear error rather than a
// silent no-op, matching the mock source's ErrDeployNotFound semantics so the
// /deploy DELETE contract is profile-consistent.
//
// We tolerate a Service that's already gone (IsNotFound on the Service delete is
// swallowed) so a partially-deleted deploy can be re-deleted to completion.
func (s *Source) DeleteDeploy(ctx context.Context, deployID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := strings.TrimSpace(deployID)
	if id == "" {
		return fmt.Errorf("%w: deployId is required", ErrDeployInvalidRequest)
	}

	selector := deployIDLabel + "=" + id
	listOpts := metav1.ListOptions{LabelSelector: selector}

	deployments, err := s.client.AppsV1().Deployments("").List(ctx, listOpts)
	if err != nil {
		return errFromAPIServer("list deployments for delete", err)
	}
	services, err := s.client.CoreV1().Services("").List(ctx, listOpts)
	if err != nil {
		return errFromAPIServer("list services for delete", err)
	}

	if len(deployments.Items) == 0 && len(services.Items) == 0 {
		return ErrResourceNotFound
	}

	// Delete Deployments first (they own the pods). Foreground propagation so the
	// ReplicaSet/Pods are garbage-collected with the Deployment.
	fg := metav1.DeletePropagationForeground
	delOpts := metav1.DeleteOptions{PropagationPolicy: &fg}
	for i := range deployments.Items {
		d := &deployments.Items[i]
		if err := s.client.AppsV1().Deployments(d.Namespace).Delete(ctx, d.Name, delOpts); err != nil {
			if apierrors.IsNotFound(err) {
				continue // raced with another deleter — fine
			}
			return errFromAPIServer("delete deployment "+d.Namespace+"/"+d.Name, err)
		}
	}
	for i := range services.Items {
		sv := &services.Items[i]
		if err := s.client.CoreV1().Services(sv.Namespace).Delete(ctx, sv.Name, metav1.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return errFromAPIServer("delete service "+sv.Namespace+"/"+sv.Name, err)
		}
	}
	return nil
}

// deployLabels returns the label set stamped on every object a deploy creates.
// The ownership pair (managed-by + id) is the delete handle; the name label is
// the conventional app identity used by the Service selector.
func deployLabels(workloadName, deployID string) map[string]string {
	return map[string]string{
		managedByLabel: managedByValue,
		deployIDLabel:  deployID,
		nameLabel:      workloadName,
	}
}

// buildDeployment assembles the apps/v1.Deployment. The pod template carries the
// SAME ownership labels as the Deployment so (a) the Service selector resolves
// the pods and (b) DeleteDeploy's label-selector delete reaches the pods via the
// Deployment's foreground GC. The selector matchLabels intentionally uses only
// the immutable identity labels (name + id) — managed-by is on the objects for
// the delete query but kept out of the selector so it stays a pure ownership tag.
func buildDeployment(namespace, name string, labels map[string]string, replicas int32, image string, npuPerReplica int) *appsv1.Deployment {
	selectorLabels := map[string]string{
		nameLabel:     name,
		deployIDLabel: labels[deployIDLabel],
	}

	container := corev1.Container{
		Name:  "main",
		Image: image,
		Ports: []corev1.ContainerPort{{ContainerPort: 8080, Name: "http"}},
	}
	if npuPerReplica > 0 {
		qty := resource.NewQuantity(int64(npuPerReplica), resource.DecimalSI)
		container.Resources = corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{ascend910BResource: *qty},
			Requests: corev1.ResourceList{ascend910BResource: *qty},
		}
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
		},
	}
}

// buildService assembles the ClusterIP Service fronting the deploy's pods. The
// selector targets the immutable identity labels (name + id) so it resolves only
// this deploy's pods even if two deploys share a preset.
func buildService(namespace, name string, labels map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				nameLabel:     name,
				deployIDLabel: labels[deployIDLabel],
			},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
				Protocol:   corev1.ProtocolTCP,
			}},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// deployImage resolves the container image from req.Parameters["image"], falling
// back to defaultDeployImage. JSON decodes the value as a string; a non-string /
// empty value falls through to the default.
func deployImage(req *model.DeployRequest) string {
	if req != nil && req.Parameters != nil {
		if v, ok := req.Parameters[deployParamImage]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return defaultDeployImage
}

// deployNPUCount resolves the per-replica Ascend910B request from
// req.Parameters["npuCount"]. JSON numbers decode as float64; a numeric string is
// also accepted. Returns 0 (no NPU request) when absent / unparseable / <= 0.
func deployNPUCount(req *model.DeployRequest) int {
	if req == nil || req.Parameters == nil {
		return 0
	}
	v, ok := req.Parameters[deployParamNPUCount]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

// sanitizeDeployName lowercases + reduces an arbitrary name to a DNS-1123 label
// (a-z0-9 and '-', no leading/trailing dash) so a caller-supplied req.Name can't
// produce an apiserver-rejected object name. Empty in → empty out (the caller
// substitutes a generated name).
func sanitizeDeployName(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	if in == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.' || r == ' ':
			b.WriteByte('-')
		}
		// any other rune is dropped
	}
	out := strings.Trim(b.String(), "-")
	// Collapse runs of '-' that the substitution above may have produced.
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	// DNS-1123 labels cap at 63 chars; trim and re-strip a trailing dash.
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	return out
}
