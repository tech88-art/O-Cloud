package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

func int32Ptr(i int32) *int32 { return &i }

// mkDeployment builds a minimal Deployment for tests. Status flips
// "running" when ready==desired; "pending" otherwise.
func mkDeployment(ns, name string, desired, ready int32, labels map[string]string) *appsv1.Deployment {
	if labels == nil {
		labels = map[string]string{}
	}
	avail := corev1.ConditionFalse
	if ready == desired && desired > 0 {
		avail = corev1.ConditionTrue
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(desired),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: ready,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: avail},
			},
		},
	}
}

func mkStatefulSet(ns, name string, desired, ready int32, labels map[string]string) *appsv1.StatefulSet {
	if labels == nil {
		labels = map[string]string{}
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(desired),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: ready},
	}
}

func mkJob(ns, name string, succeeded, active int32, condType batchv1.JobConditionType, condStatus corev1.ConditionStatus, labels map[string]string) *batchv1.Job {
	if labels == nil {
		labels = map[string]string{}
	}
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: batchv1.JobSpec{
			Completions: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"job-name": name},
			},
		},
		Status: batchv1.JobStatus{Succeeded: succeeded, Active: active},
	}
	if condType != "" {
		j.Status.Conditions = []batchv1.JobCondition{
			{Type: condType, Status: condStatus},
		}
	}
	return j
}

func mkPod(ns, name, node string, phase corev1.PodPhase, labels, annotations map[string]string) *corev1.Pod {
	if labels == nil {
		labels = map[string]string{}
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			NodeName: node,
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "test/image:latest",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:                   resource.MustParse("16"),
							corev1.ResourceMemory:                resource.MustParse("64Gi"),
							corev1.ResourceName("huawei.com/Ascend910B"): *resource.NewQuantity(1, resource.DecimalSI),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func TestListWorkloads_AllKinds_StableOrder(t *testing.T) {
	client := fake.NewSimpleClientset(
		mkDeployment("ai-inference", "qwen-8b", 2, 2, nil),
		mkStatefulSet("ai-inference", "alpha-cache", 1, 1, nil),
		mkJob("benchmark", "vllm-bench", 0, 1, "", "", nil),
	)
	src := NewSourceWithClient(client, Options{})

	out, err := src.ListWorkloads(context.Background(), model.WorkloadFilter{})
	require.NoError(t, err)
	require.Len(t, out, 3)

	// Stable order: (namespace, name) lexicographic.
	assert.Equal(t, "ai-inference/alpha-cache", out[0].Namespace+"/"+out[0].Name)
	assert.Equal(t, "ai-inference/qwen-8b", out[1].Namespace+"/"+out[1].Name)
	assert.Equal(t, "benchmark/vllm-bench", out[2].Namespace+"/"+out[2].Name)

	// Kind mapping check.
	kinds := map[string]string{}
	for _, w := range out {
		kinds[w.Name] = w.Kind
	}
	assert.Equal(t, "Deployment", kinds["qwen-8b"])
	assert.Equal(t, "StatefulSet", kinds["alpha-cache"])
	assert.Equal(t, "Job", kinds["vllm-bench"])
}

func TestListWorkloads_FilterByNamespace(t *testing.T) {
	client := fake.NewSimpleClientset(
		mkDeployment("ai-inference", "qwen-8b", 1, 1, nil),
		mkDeployment("training", "llama-finetune", 1, 0, nil),
	)
	src := NewSourceWithClient(client, Options{})

	out, err := src.ListWorkloads(context.Background(), model.WorkloadFilter{Namespace: "training"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "llama-finetune", out[0].Name)
}

func TestListWorkloads_FilterByStatus(t *testing.T) {
	client := fake.NewSimpleClientset(
		mkDeployment("ns", "running-1", 1, 1, nil), // running
		mkDeployment("ns", "pending-1", 2, 0, nil), // pending
	)
	src := NewSourceWithClient(client, Options{})

	out, err := src.ListWorkloads(context.Background(), model.WorkloadFilter{Status: "running"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "running-1", out[0].Name)
}

func TestListWorkloads_FilterByType_FromLabel(t *testing.T) {
	client := fake.NewSimpleClientset(
		mkDeployment("ns", "infer-1", 1, 1, map[string]string{"huawei.com/workload-type": "inference"}),
		mkDeployment("ns", "bench-1", 1, 1, map[string]string{"huawei.com/workload-type": "benchmark"}),
	)
	src := NewSourceWithClient(client, Options{})

	out, err := src.ListWorkloads(context.Background(), model.WorkloadFilter{Type: "BENCHMARK"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "bench-1", out[0].Name)
}

func TestGetWorkloadDetail_Deployment_HappyPath_WithPods(t *testing.T) {
	dep := mkDeployment("ai-inference", "qwen-8b", 2, 2, nil)
	p1 := mkPod("ai-inference", "qwen-8b-0", "worker-1", corev1.PodRunning,
		map[string]string{"app.kubernetes.io/name": "qwen-8b"}, nil)
	p2 := mkPod("ai-inference", "qwen-8b-1", "worker-2", corev1.PodRunning,
		map[string]string{"app.kubernetes.io/name": "qwen-8b"}, nil)
	client := fake.NewSimpleClientset(dep, p1, p2)
	src := NewSourceWithClient(client, Options{})

	detail, err := src.GetWorkloadDetail(context.Background(), "ai-inference", "qwen-8b")
	require.NoError(t, err)
	assert.Equal(t, "qwen-8b", detail.Name)
	assert.Equal(t, "Deployment", detail.Kind)
	require.Len(t, detail.Pods, 2)
	assert.Equal(t, "qwen-8b-0", detail.Pods[0].Name)
	assert.Equal(t, "qwen-8b-1", detail.Pods[1].Name)
	// Per-container resource projection — Ascend request surfaced
	// as NPUSlices placeholder ids.
	require.NotNil(t, detail.Pods[0].Containers[0].Resources)
	assert.Equal(t, "16", detail.Pods[0].Containers[0].Resources.CPU)
	require.Len(t, detail.Pods[0].Containers[0].Resources.NPUSlices, 1)
}

func TestGetWorkloadDetail_PDPair_RelationsDerived(t *testing.T) {
	dep := mkDeployment("ai-inference", "qwen-8b-pd", 2, 2, nil)
	pre := mkPod("ai-inference", "qwen-8b-pd-prefill-0", "worker-1", corev1.PodRunning,
		map[string]string{
			"app.kubernetes.io/name":     "qwen-8b-pd",
			"huawei.com/inference-role":  "prefill",
			"huawei.com/pd-pair-id":      "pair-001",
		}, nil)
	dec := mkPod("ai-inference", "qwen-8b-pd-decode-0", "worker-2", corev1.PodRunning,
		map[string]string{
			"app.kubernetes.io/name":     "qwen-8b-pd",
			"huawei.com/inference-role":  "decode",
			"huawei.com/pd-pair-id":      "pair-001",
		}, nil)
	client := fake.NewSimpleClientset(dep, pre, dec)
	src := NewSourceWithClient(client, Options{})

	detail, err := src.GetWorkloadDetail(context.Background(), "ai-inference", "qwen-8b-pd")
	require.NoError(t, err)
	require.Len(t, detail.Relations, 1)
	assert.Equal(t, "qwen-8b-pd-prefill-0", detail.Relations[0].From)
	assert.Equal(t, "qwen-8b-pd-decode-0", detail.Relations[0].To)
	assert.Equal(t, "pd-pair", detail.Relations[0].Type)
}

func TestGetWorkloadDetail_StatefulSet(t *testing.T) {
	ss := mkStatefulSet("data", "kv-cache", 3, 3, nil)
	client := fake.NewSimpleClientset(ss)
	src := NewSourceWithClient(client, Options{})

	detail, err := src.GetWorkloadDetail(context.Background(), "data", "kv-cache")
	require.NoError(t, err)
	assert.Equal(t, "StatefulSet", detail.Kind)
	assert.Equal(t, "running", detail.Status)
}

func TestGetWorkloadDetail_Job_Succeeded(t *testing.T) {
	j := mkJob("benchmark", "vllm-bench", 1, 0, batchv1.JobComplete, corev1.ConditionTrue, nil)
	client := fake.NewSimpleClientset(j)
	src := NewSourceWithClient(client, Options{})

	detail, err := src.GetWorkloadDetail(context.Background(), "benchmark", "vllm-bench")
	require.NoError(t, err)
	assert.Equal(t, "Job", detail.Kind)
	assert.Equal(t, "succeeded", detail.Status)
}

func TestGetWorkloadDetail_NotFound(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	_, err := src.GetWorkloadDetail(context.Background(), "ns", "no-such")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestGetWorkloadDetail_EmptyNS_or_Name(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	_, err := src.GetWorkloadDetail(context.Background(), "", "x")
	assert.ErrorIs(t, err, ErrResourceNotFound)
	_, err = src.GetWorkloadDetail(context.Background(), "ns", "")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestParseSliceBindingsAnnotation_SemicolonForm(t *testing.T) {
	in := map[string]string{
		"npu.huawei.com/slice-bindings": "sliceId=worker-a-01-npu-0,role=prefill,indexInPod=0;sliceId=worker-a-01-npu-1,role=decode,indexInPod=0",
	}
	got := parseSliceBindingsAnnotation(in)
	require.Len(t, got, 2)
	assert.Equal(t, "worker-a-01-npu-0", got[0].SliceID)
	assert.Equal(t, "prefill", got[0].Role)
	assert.Equal(t, 0, got[0].IndexInPod)
	assert.Equal(t, "worker-a-01-npu-1", got[1].SliceID)
}

func TestParseSliceBindingsAnnotation_AbsentReturnsNil(t *testing.T) {
	assert.Nil(t, parseSliceBindingsAnnotation(nil))
	assert.Nil(t, parseSliceBindingsAnnotation(map[string]string{}))
	assert.Nil(t, parseSliceBindingsAnnotation(map[string]string{
		"npu.huawei.com/slice-bindings": "",
	}))
}

func TestParseSliceBindingsAnnotation_JSONFormDeferred(t *testing.T) {
	// JSON form is reserved for P2-T-105; current impl silently ignores
	// JSON-looking annotations to avoid half-parsed data.
	in := map[string]string{
		"npu.huawei.com/slice-bindings": `[{"sliceId":"foo","role":"prefill"}]`,
	}
	got := parseSliceBindingsAnnotation(in)
	assert.Nil(t, got)
}

func TestDeriveWorkloadType_FallbackOther(t *testing.T) {
	assert.Equal(t, "other", deriveWorkloadType(nil, nil))
	assert.Equal(t, "other", deriveWorkloadType(map[string]string{"foo": "bar"}, nil))
}

func TestDeriveWorkloadType_PrefersHuaweiLabel(t *testing.T) {
	labels := map[string]string{
		"huawei.com/workload-type":  "training",
		"app.kubernetes.io/component": "inference",
	}
	assert.Equal(t, "training", deriveWorkloadType(labels, nil))
}
