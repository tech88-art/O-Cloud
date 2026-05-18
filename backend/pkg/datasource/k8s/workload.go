package k8s

import (
	"context"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Workload sources we project onto model.Workload.
//
// Phase 2 covers the three kinds the demo cares about:
//   - apps/v1.Deployment    → kind="Deployment", type heuristic by labels
//   - apps/v1.StatefulSet   → kind="StatefulSet"
//   - batch/v1.Job          → kind="Job", type often "benchmark"|"training"
//
// We intentionally leave CronJob / DaemonSet out — the demo doesn't
// expose those on the Workloads page, and adding them would force a
// scoping question (DaemonSets that aren't "workloads" in the
// operator sense). Phase 3+ can extend if needed.
//
// PD-pair detection (`relations[].type=pd-pair`) reads two
// well-known labels:
//
//   - huawei.com/inference-role  ∈ {prefill, decode}
//   - huawei.com/pd-pair-id      = shared id across the two pods
//
// Pods sharing the same `pd-pair-id` AND opposite roles emit one
// PodRelation each in the workload's `relations[]`. The label
// convention was agreed in ADR-0005 Phase-2 §"Bindings"; this
// implementation surfaces it client-side until the K8s scheduler
// extender lands.

// ListWorkloads aggregates Deployments + StatefulSets + Jobs into the
// model.Workload list-view. Filter semantics match WorkloadFilter:
//   - Namespace empty → all namespaces
//   - Type     empty → all types; non-empty filters by derived type
//   - Status   empty → all statuses; non-empty filters by derived status
//
// All projection helpers below are pure; the apiserver list calls are
// the only side effect.
func (s *Source) ListWorkloads(ctx context.Context, filter model.WorkloadFilter) ([]*model.Workload, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ns := filter.Namespace // "" means cross-namespace

	deployments, err := s.client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errFromAPIServer("list deployments", err)
	}
	statefulSets, err := s.client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errFromAPIServer("list statefulsets", err)
	}
	jobs, err := s.client.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errFromAPIServer("list jobs", err)
	}

	out := make([]*model.Workload, 0,
		len(deployments.Items)+len(statefulSets.Items)+len(jobs.Items))

	for i := range deployments.Items {
		w := projectDeployment(&deployments.Items[i])
		if matchesWorkloadFilter(w, filter) {
			out = append(out, w)
		}
	}
	for i := range statefulSets.Items {
		w := projectStatefulSet(&statefulSets.Items[i])
		if matchesWorkloadFilter(w, filter) {
			out = append(out, w)
		}
	}
	for i := range jobs.Items {
		w := projectJob(&jobs.Items[i])
		if matchesWorkloadFilter(w, filter) {
			out = append(out, w)
		}
	}

	// Stable order: (namespace, name). The handler emits the JSON as-is
	// so two consecutive list requests return byte-identical bodies
	// when the cluster hasn't changed.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// GetWorkloadDetail aggregates a single workload + its pods +
// relations. Discovers the kind via a series of Get calls (Deployment
// → StatefulSet → Job); the first one that resolves wins. Returns
// ErrResourceNotFound when none match.
func (s *Source) GetWorkloadDetail(ctx context.Context, namespace, name string) (*model.WorkloadDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if namespace == "" || name == "" {
		return nil, ErrResourceNotFound
	}

	var (
		base     *model.Workload
		selector labels  // see selector.go below — minimal map[string]string wrap
	)

	// Try Deployment first.
	if d, err := s.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		base = projectDeployment(d)
		selector = labels(d.Spec.Selector.MatchLabels)
	} else if !apierrors.IsNotFound(err) {
		return nil, errFromAPIServer("get deployment", err)
	}

	if base == nil {
		if ss, err := s.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
			base = projectStatefulSet(ss)
			selector = labels(ss.Spec.Selector.MatchLabels)
		} else if !apierrors.IsNotFound(err) {
			return nil, errFromAPIServer("get statefulset", err)
		}
	}

	if base == nil {
		if j, err := s.client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
			base = projectJob(j)
			selector = labels(j.Spec.Selector.MatchLabels)
		} else if !apierrors.IsNotFound(err) {
			return nil, errFromAPIServer("get job", err)
		}
	}

	if base == nil {
		return nil, ErrResourceNotFound
	}

	// Discover pods via the workload's selector (label-set matching).
	podList, err := s.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.asLabelSelector(),
	})
	if err != nil {
		return nil, errFromAPIServer("list workload pods", err)
	}

	pods := make([]model.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pods = append(pods, projectPod(&podList.Items[i]))
	}
	sort.SliceStable(pods, func(i, j int) bool { return pods[i].Name < pods[j].Name })

	relations := derivePDPairRelations(pods, podList.Items)

	detail := &model.WorkloadDetail{
		Workload:  *base,
		Pods:      pods,
		Relations: relations,
	}
	return detail, nil
}

// ------------------------------------------------------------------ projections

// projectDeployment builds the list-view model.Workload from a
// Deployment. Status is derived from the Available condition; "type"
// is heuristic (label-driven, see deriveWorkloadType).
func projectDeployment(d *appsv1.Deployment) *model.Workload {
	w := &model.Workload{
		Name:      d.Name,
		Namespace: d.Namespace,
		Type:      deriveWorkloadType(d.Labels, d.Annotations),
		Kind:      "Deployment",
		Status:    deploymentStatus(d),
		Labels:    d.Labels,
	}
	if d.Spec.Replicas != nil {
		w.Replicas = &model.ReplicaStatus{
			Desired: int(*d.Spec.Replicas),
			Ready:   int(d.Status.ReadyReplicas),
		}
	}
	stamp := d.CreationTimestamp.UTC()
	w.CreatedAt = &stamp
	return w
}

func projectStatefulSet(ss *appsv1.StatefulSet) *model.Workload {
	w := &model.Workload{
		Name:      ss.Name,
		Namespace: ss.Namespace,
		Type:      deriveWorkloadType(ss.Labels, ss.Annotations),
		Kind:      "StatefulSet",
		Status:    statefulSetStatus(ss),
		Labels:    ss.Labels,
	}
	if ss.Spec.Replicas != nil {
		w.Replicas = &model.ReplicaStatus{
			Desired: int(*ss.Spec.Replicas),
			Ready:   int(ss.Status.ReadyReplicas),
		}
	}
	stamp := ss.CreationTimestamp.UTC()
	w.CreatedAt = &stamp
	return w
}

func projectJob(j *batchv1.Job) *model.Workload {
	w := &model.Workload{
		Name:      j.Name,
		Namespace: j.Namespace,
		Type:      deriveWorkloadType(j.Labels, j.Annotations),
		Kind:      "Job",
		Status:    jobStatus(j),
		Labels:    j.Labels,
	}
	desired := int32(1)
	if j.Spec.Completions != nil {
		desired = *j.Spec.Completions
	}
	w.Replicas = &model.ReplicaStatus{
		Desired: int(desired),
		Ready:   int(j.Status.Succeeded + j.Status.Active),
	}
	stamp := j.CreationTimestamp.UTC()
	w.CreatedAt = &stamp
	return w
}

// projectPod surfaces the model.Pod from a corev1.Pod. Containers +
// resources + npuSlices come from the pod spec; bindings come from
// the `npu.huawei.com/slice-bindings` annotation if present
// (ADR-0005 Phase 2 — P2-T-105 will tighten the parser).
func projectPod(p *corev1.Pod) model.Pod {
	out := model.Pod{
		Name:      p.Name,
		Namespace: p.Namespace,
		NodeName:  p.Spec.NodeName,
		Status:    string(p.Status.Phase),
	}
	for _, c := range p.Spec.Containers {
		mc := model.Container{
			Name:    c.Name,
			Image:   c.Image,
			Command: c.Command,
			Args:    c.Args,
		}
		// Surface NPU slice requests as resource hints. We carry only
		// the requests/limits keys with the huawei.com/Ascend prefix;
		// CPU/Memory live in their own fields.
		if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			mc.Resources = &model.ContainerResources{CPU: cpu.String()}
		}
		if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			if mc.Resources == nil {
				mc.Resources = &model.ContainerResources{}
			}
			mc.Resources.Memory = mem.String()
		}
		for k, q := range c.Resources.Requests {
			key := string(k)
			if !strings.HasPrefix(key, ascendResourcePrefix) {
				continue
			}
			if mc.Resources == nil {
				mc.Resources = &model.ContainerResources{}
			}
			// Surface the request count as a "n × <model>" slice id list.
			// P2-T-105 will resolve to concrete slice ids via scheduler
			// annotations; for now the list captures intent.
			if n, ok := q.AsInt64(); ok {
				modelName := strings.TrimPrefix(key, "huawei.com/")
				for i := int64(0); i < n; i++ {
					mc.Resources.NPUSlices = append(mc.Resources.NPUSlices,
						modelName+"-request-"+strconv.FormatInt(i, 10))
				}
			}
		}
		out.Containers = append(out.Containers, mc)
	}
	out.Bindings = parseSliceBindingsAnnotation(p.Annotations)
	return out
}

// ------------------------------------------------------------------ status helpers

// deploymentStatus folds the Available condition into the Phase 1
// status vocabulary.
func deploymentStatus(d *appsv1.Deployment) string {
	for _, c := range d.Status.Conditions {
		if c.Type != appsv1.DeploymentAvailable {
			continue
		}
		if c.Status == corev1.ConditionTrue {
			return "running"
		}
		return "pending"
	}
	if d.Spec.Replicas != nil && d.Status.ReadyReplicas == *d.Spec.Replicas {
		return "running"
	}
	return "pending"
}

func statefulSetStatus(ss *appsv1.StatefulSet) string {
	if ss.Spec.Replicas != nil && ss.Status.ReadyReplicas == *ss.Spec.Replicas {
		return "running"
	}
	return "pending"
}

func jobStatus(j *batchv1.Job) string {
	for _, c := range j.Status.Conditions {
		switch c.Type {
		case batchv1.JobComplete:
			if c.Status == corev1.ConditionTrue {
				return "succeeded"
			}
		case batchv1.JobFailed:
			if c.Status == corev1.ConditionTrue {
				return "failed"
			}
		}
	}
	if j.Status.Active > 0 {
		return "running"
	}
	if j.Status.Succeeded > 0 {
		return "succeeded"
	}
	return "pending"
}

// ------------------------------------------------------------------ type heuristic

// deriveWorkloadType reads `app.kubernetes.io/component` then
// `huawei.com/workload-type` then falls back to "other". The Phase 1
// vocabulary is inference / benchmark / training / other.
func deriveWorkloadType(labels, annotations map[string]string) string {
	for _, key := range []string{
		"huawei.com/workload-type",
		"app.kubernetes.io/component",
	} {
		if v, ok := labels[key]; ok && v != "" {
			return v
		}
		if v, ok := annotations[key]; ok && v != "" {
			return v
		}
	}
	return "other"
}

// ------------------------------------------------------------------ filter

func matchesWorkloadFilter(w *model.Workload, f model.WorkloadFilter) bool {
	if f.Namespace != "" && w.Namespace != f.Namespace {
		return false
	}
	if f.Type != "" && !equalFold(w.Type, f.Type) {
		return false
	}
	if f.Status != "" && !equalFold(w.Status, f.Status) {
		return false
	}
	return true
}

// ------------------------------------------------------------------ selector

// labels is a minimal helper that wraps map[string]string and yields
// a comma-separated `key=value` label selector. The k8s.io
// labels.Selector type would also work but pulls a wider import surface
// we don't need for this single use case.
type labels map[string]string

func (l labels) asLabelSelector() string {
	if len(l) == 0 {
		return ""
	}
	keys := make([]string, 0, len(l))
	for k := range l {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+l[k])
	}
	return strings.Join(parts, ",")
}

// ------------------------------------------------------------------ PD-pair

const (
	labelInferenceRole = "huawei.com/inference-role"
	labelPDPairID      = "huawei.com/pd-pair-id"
)

// derivePDPairRelations groups pods by pd-pair-id label and emits one
// PodRelation per (prefill, decode) pair. Missing role / mismatched
// pair / single-pod groups all silently skip — the relation list is
// "what we KNOW is a PD pair", not "what might be one".
func derivePDPairRelations(_ []model.Pod, pods []corev1.Pod) []model.PodRelation {
	groups := map[string]map[string]string{} // pairId → role → podName
	for i := range pods {
		p := &pods[i]
		role := p.Labels[labelInferenceRole]
		pairID := p.Labels[labelPDPairID]
		if role == "" || pairID == "" {
			continue
		}
		if groups[pairID] == nil {
			groups[pairID] = map[string]string{}
		}
		groups[pairID][role] = p.Name
	}

	pairIDs := make([]string, 0, len(groups))
	for k := range groups {
		pairIDs = append(pairIDs, k)
	}
	sort.Strings(pairIDs)

	var rels []model.PodRelation
	for _, id := range pairIDs {
		g := groups[id]
		pre, hasPre := g["prefill"]
		dec, hasDec := g["decode"]
		if !hasPre || !hasDec {
			continue
		}
		rels = append(rels, model.PodRelation{
			From: pre,
			To:   dec,
			Type: "pd-pair",
		})
	}
	return rels
}

// ------------------------------------------------------------------ bindings

// parseSliceBindingsAnnotation reads `npu.huawei.com/slice-bindings`
// — a JSON-encoded array of {sliceId, role, indexInPod}. P2-T-105
// will tighten this with stricter validation + a richer schema; here
// we accept the canonical form and silently drop malformed entries.
//
// Returns nil when the annotation is absent / blank / malformed —
// callers must defend against a nil slice (model.Pod.Bindings is
// json-omitempty, so nil round-trips as absent).
func parseSliceBindingsAnnotation(annotations map[string]string) []model.PodBinding {
	raw := annotations["npu.huawei.com/slice-bindings"]
	if raw == "" {
		return nil
	}
	// Light-touch parser: the annotation is conventionally JSON, but to
	// avoid pulling encoding/json (this is already in the import set
	// elsewhere, but keep the function focused) we accept either
	// canonical JSON OR a semicolon-separated key=value form for the
	// hand-edited annotations operators sometimes apply directly. P2-T-
	// 105 promotes this to the strict JSON-only path.
	out := []model.PodBinding{}
	if raw[0] == '[' {
		// JSON path — wired in P2-T-105 with encoding/json; for P2-T-003
		// we treat the JSON form as "leave for P2-T-105" and skip it
		// silently. The semicolon form below is the placeholder.
		return nil
	}
	for _, chunk := range strings.Split(raw, ";") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		entry := model.PodBinding{}
		for _, kv := range strings.Split(chunk, ",") {
			kv = strings.TrimSpace(kv)
			eq := strings.IndexByte(kv, '=')
			if eq <= 0 {
				continue
			}
			key := kv[:eq]
			val := kv[eq+1:]
			switch key {
			case "sliceId":
				entry.SliceID = val
			case "role":
				entry.Role = val
			case "indexInPod":
				if n, err := strconv.Atoi(val); err == nil {
					entry.IndexInPod = n
				}
			}
		}
		if entry.SliceID == "" {
			continue
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
