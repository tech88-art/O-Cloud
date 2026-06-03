package k8s

// k8s.Source real Deploy() / DeleteDeploy() tests (P13-T-104). Drive the apply +
// delete paths off a kubernetes/fake clientset (no real cluster, per ADR-0024 §3
// "captured-fixture test 通"). Assert the Deployment + Service materialise with
// the right name/namespace/labels/replicas/image, that DeleteDeploy removes them
// by label selector, the round-trip, and unknown-id delete behaviour.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

func newDeploySource() *Source {
	return NewSourceWithClient(fake.NewSimpleClientset(), Options{ClusterIDOverride: "demo"})
}

func TestDeploy_NoLongerErrCapabilityUnavailable(t *testing.T) {
	// Headline of P13-T-104: Deploy must not be a stub.
	src := newDeploySource()
	resp, err := src.Deploy(context.Background(), &model.DeployRequest{
		PresetID:  "qwen-8b",
		Namespace: "ocloud-workloads",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotErrorIs(t, err, datasource.ErrCapabilityUnavailable)
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEmpty(t, resp.DeployID)
}

func TestDeploy_MaterializesDeploymentAndService(t *testing.T) {
	src := newDeploySource()
	resp, err := src.Deploy(context.Background(), &model.DeployRequest{
		PresetID:  "qwen-8b",
		Namespace: "ocloud-workloads",
		Name:      "my-infer",
		Replicas:  3,
		Parameters: map[string]interface{}{
			"image":    "registry.example.com/vllm-ascend:v0.11.0",
			"npuCount": float64(2), // JSON numbers decode as float64
		},
	})
	require.NoError(t, err)
	require.Equal(t, "my-infer", resp.WorkloadName)
	require.Equal(t, "my-infer", resp.DeployID)
	require.Equal(t, "ocloud-workloads", resp.Namespace)

	// Deployment materialised with the requested shape.
	dep, err := src.client.AppsV1().Deployments("ocloud-workloads").Get(
		context.Background(), "my-infer", metav1.GetOptions{})
	require.NoError(t, err, "Deployment must exist on the apiserver")
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(3), *dep.Spec.Replicas)

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "registry.example.com/vllm-ascend:v0.11.0", c.Image)

	// NPU resource request stamped from npuCount=2.
	npuReq := c.Resources.Requests[ascend910BResource]
	assert.Equal(t, int64(2), npuReq.Value(), "huawei.com/Ascend910B request == npuCount")
	npuLim := c.Resources.Limits[ascend910BResource]
	assert.Equal(t, int64(2), npuLim.Value())

	// Ownership labels on the Deployment + the pod template.
	assert.Equal(t, managedByValue, dep.Labels[managedByLabel])
	assert.Equal(t, "my-infer", dep.Labels[deployIDLabel])
	assert.Equal(t, managedByValue, dep.Spec.Template.Labels[managedByLabel])
	assert.Equal(t, "my-infer", dep.Spec.Template.Labels[deployIDLabel])

	// Service materialised, selecting the pod identity labels.
	svc, err := src.client.CoreV1().Services("ocloud-workloads").Get(
		context.Background(), "my-infer", metav1.GetOptions{})
	require.NoError(t, err, "Service must exist on the apiserver")
	assert.Equal(t, "my-infer", svc.Spec.Selector[nameLabel])
	assert.Equal(t, "my-infer", svc.Spec.Selector[deployIDLabel])
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(8080), svc.Spec.Ports[0].Port)
}

func TestDeploy_DefaultsImageAndReplicasAndNoNPU(t *testing.T) {
	src := newDeploySource()
	resp, err := src.Deploy(context.Background(), &model.DeployRequest{
		PresetID:  "smoke",
		Namespace: "default",
		Name:      "bare",
		// no replicas → default 1; no image → defaultDeployImage; no npuCount → no request
	})
	require.NoError(t, err)

	dep, err := src.client.AppsV1().Deployments("default").Get(
		context.Background(), resp.WorkloadName, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas, "replicas defaults to 1")

	c := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, defaultDeployImage, c.Image, "image defaults to vllm-ascend")
	assert.Empty(t, c.Resources.Requests, "no npuCount → no resource request")
	assert.Empty(t, c.Resources.Limits)
}

func TestDeploy_GeneratesNameWhenAbsent(t *testing.T) {
	src := newDeploySource()
	resp, err := src.Deploy(context.Background(), &model.DeployRequest{
		PresetID:  "qwen-8b",
		Namespace: "default",
		// no Name → generated "<presetId>-<unixnano>"
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.WorkloadName)
	assert.Contains(t, resp.WorkloadName, "qwen-8b-", "generated name carries presetId prefix")

	// And the generated name actually backs a real object.
	_, err = src.client.AppsV1().Deployments("default").Get(
		context.Background(), resp.WorkloadName, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestDeploy_SanitizesCallerSuppliedName(t *testing.T) {
	src := newDeploySource()
	resp, err := src.Deploy(context.Background(), &model.DeployRequest{
		PresetID:  "qwen-8b",
		Namespace: "default",
		Name:      "My Infer_Svc.01", // upper + space + underscore + dot
	})
	require.NoError(t, err)
	// Lowercased, separators → '-', collapsed, no leading/trailing dash.
	assert.Equal(t, "my-infer-svc-01", resp.WorkloadName)
	_, err = src.client.AppsV1().Deployments("default").Get(
		context.Background(), "my-infer-svc-01", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestDeploy_ValidationErrors(t *testing.T) {
	src := newDeploySource()

	_, err := src.Deploy(context.Background(), nil)
	assert.ErrorIs(t, err, ErrDeployInvalidRequest, "nil request")

	_, err = src.Deploy(context.Background(), &model.DeployRequest{Namespace: "default"})
	assert.ErrorIs(t, err, ErrDeployInvalidRequest, "missing presetId")

	_, err = src.Deploy(context.Background(), &model.DeployRequest{PresetID: "x"})
	assert.ErrorIs(t, err, ErrDeployInvalidRequest, "missing namespace")

	_, err = src.Deploy(context.Background(), &model.DeployRequest{
		PresetID: "x", Namespace: "default", Replicas: -2,
	})
	assert.ErrorIs(t, err, ErrDeployInvalidRequest, "negative replicas")
}

func TestDeploy_DuplicateNameConflicts(t *testing.T) {
	src := newDeploySource()
	req := &model.DeployRequest{PresetID: "qwen-8b", Namespace: "default", Name: "dup"}
	_, err := src.Deploy(context.Background(), req)
	require.NoError(t, err)

	_, err = src.Deploy(context.Background(), req)
	assert.ErrorIs(t, err, ErrDeployConflict, "re-deploying the same name is a conflict")
}

func TestDeleteDeploy_RemovesOwnedObjects(t *testing.T) {
	src := newDeploySource()
	resp, err := src.Deploy(context.Background(), &model.DeployRequest{
		PresetID: "qwen-8b", Namespace: "ocloud-workloads", Name: "to-delete",
	})
	require.NoError(t, err)

	// Pre-condition: both objects exist.
	_, err = src.client.AppsV1().Deployments("ocloud-workloads").Get(
		context.Background(), "to-delete", metav1.GetOptions{})
	require.NoError(t, err)
	_, err = src.client.CoreV1().Services("ocloud-workloads").Get(
		context.Background(), "to-delete", metav1.GetOptions{})
	require.NoError(t, err)

	require.NoError(t, src.DeleteDeploy(context.Background(), resp.DeployID))

	// Both gone.
	_, err = src.client.AppsV1().Deployments("ocloud-workloads").Get(
		context.Background(), "to-delete", metav1.GetOptions{})
	assert.Error(t, err, "Deployment must be deleted")
	_, err = src.client.CoreV1().Services("ocloud-workloads").Get(
		context.Background(), "to-delete", metav1.GetOptions{})
	assert.Error(t, err, "Service must be deleted")
}

func TestDeploy_DeleteRoundTrip(t *testing.T) {
	src := newDeploySource()
	req := &model.DeployRequest{PresetID: "qwen-8b", Namespace: "default", Name: "rt"}

	resp, err := src.Deploy(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, src.DeleteDeploy(context.Background(), resp.DeployID))

	// After delete, the name is free → re-deploy succeeds (no lingering conflict).
	_, err = src.Deploy(context.Background(), req)
	assert.NoError(t, err, "name is reusable after delete (round-trip)")
}

func TestDeleteDeploy_UnknownIDIsNotFound(t *testing.T) {
	src := newDeploySource()
	err := src.DeleteDeploy(context.Background(), "does-not-exist")
	assert.ErrorIs(t, err, ErrResourceNotFound, "unknown deployId → 404 sentinel")
}

func TestDeleteDeploy_EmptyIDInvalid(t *testing.T) {
	src := newDeploySource()
	err := src.DeleteDeploy(context.Background(), "  ")
	assert.ErrorIs(t, err, ErrDeployInvalidRequest, "blank deployId is a 400-shaped error")
}

func TestDeleteDeploy_OnlySelectsOwnedID(t *testing.T) {
	// Two independent deploys; deleting one must leave the other untouched
	// (label-selector scoping by deploy.ocloud.io/id).
	src := newDeploySource()
	a, err := src.Deploy(context.Background(), &model.DeployRequest{
		PresetID: "qwen-8b", Namespace: "default", Name: "alpha",
	})
	require.NoError(t, err)
	_, err = src.Deploy(context.Background(), &model.DeployRequest{
		PresetID: "qwen-8b", Namespace: "default", Name: "beta",
	})
	require.NoError(t, err)

	require.NoError(t, src.DeleteDeploy(context.Background(), a.DeployID))

	// alpha gone, beta survives.
	_, err = src.client.AppsV1().Deployments("default").Get(
		context.Background(), "alpha", metav1.GetOptions{})
	assert.Error(t, err, "alpha deleted")
	_, err = src.client.AppsV1().Deployments("default").Get(
		context.Background(), "beta", metav1.GetOptions{})
	assert.NoError(t, err, "beta untouched by alpha's delete")
}

func TestDeploy_ContextCancelled(t *testing.T) {
	src := newDeploySource()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.Deploy(ctx, &model.DeployRequest{PresetID: "x", Namespace: "default"})
	assert.ErrorIs(t, err, context.Canceled)

	err = src.DeleteDeploy(ctx, "anything")
	assert.ErrorIs(t, err, context.Canceled)
}
