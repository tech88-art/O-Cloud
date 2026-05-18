package k8s

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// The fake clientset's reactor chain DOES emit watch events when
// objects are Created / Updated / Deleted via the clientset. That
// makes it usable as a test driver for StreamEvents end-to-end: we
// open the stream first, then perform mutations and assert the
// resulting WSMessages on the channel.

func TestStreamEvents_PodAddedDeleted_EmitsCreatedDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := src.StreamEvents(ctx, model.StreamEventsOptions{})
	require.NoError(t, err)

	pod := mkPod("ai-inference", "pod-1", "node-a", corev1.PodPending,
		map[string]string{"app.kubernetes.io/name": "qwen-8b"}, nil)
	_, err = client.CoreV1().Pods("ai-inference").Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err)

	msg := readNextOrTimeout(t, ch, 2*time.Second)
	require.NotNil(t, msg, "expected workload.created within 2s")
	assert.Equal(t, "workload.created", msg.Type)
	payload := decodePayload(t, msg.Payload)
	assert.Equal(t, "ai-inference", payload["namespace"])
	assert.Equal(t, "qwen-8b", payload["name"], "owner derived from app.kubernetes.io/name label")
	assert.Equal(t, "pod-1", payload["podName"])

	require.NoError(t, client.CoreV1().Pods("ai-inference").Delete(ctx, "pod-1", metav1.DeleteOptions{}))
	msg = readNextOrTimeout(t, ch, 2*time.Second)
	require.NotNil(t, msg)
	assert.Equal(t, "workload.deleted", msg.Type)
}

func TestStreamEvents_PodModified_PhaseChange_EmitsStatusChanged(t *testing.T) {
	// fake's Watch doesn't replay pre-existing objects; create after
	// the watch is opened so the watcher actually sees an Added event,
	// then mutate to drive a Modified.
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := src.StreamEvents(ctx, model.StreamEventsOptions{})
	require.NoError(t, err)

	pod := mkPod("ns", "pod-1", "node-a", corev1.PodPending,
		map[string]string{"app.kubernetes.io/name": "wl"}, nil)
	_, err = client.CoreV1().Pods("ns").Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err)
	first := readNextOrTimeout(t, ch, 2*time.Second)
	require.NotNil(t, first)
	assert.Equal(t, "workload.created", first.Type)

	// Flip phase Pending → Running; expect a workload.statusChanged.
	upd, err := client.CoreV1().Pods("ns").Get(ctx, "pod-1", metav1.GetOptions{})
	require.NoError(t, err)
	upd.Status.Phase = corev1.PodRunning
	_, err = client.CoreV1().Pods("ns").Update(ctx, upd, metav1.UpdateOptions{})
	require.NoError(t, err)

	next := readNextOrTimeout(t, ch, 2*time.Second)
	require.NotNil(t, next)
	assert.Equal(t, "workload.statusChanged", next.Type)
	p := decodePayload(t, next.Payload)
	assert.Equal(t, "Running", p["phase"])
}

func TestStreamEvents_PodModified_UnchangedPhase_NoMessage(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := src.StreamEvents(ctx, model.StreamEventsOptions{})
	require.NoError(t, err)

	pod := mkPod("ns", "pod-1", "node-a", corev1.PodRunning,
		map[string]string{"app.kubernetes.io/name": "wl"}, nil)
	_, err = client.CoreV1().Pods("ns").Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err)
	// Drain the initial ADDED.
	require.NotNil(t, readNextOrTimeout(t, ch, 2*time.Second))

	// Bump a label without changing phase; expect NO message in
	// 500ms (translatePodEvent filters phase-unchanged MODIFIED).
	upd, err := client.CoreV1().Pods("ns").Get(ctx, "pod-1", metav1.GetOptions{})
	require.NoError(t, err)
	upd.Labels["churn"] = "1"
	_, err = client.CoreV1().Pods("ns").Update(ctx, upd, metav1.UpdateOptions{})
	require.NoError(t, err)

	select {
	case m := <-ch:
		t.Fatalf("unexpected message for phase-unchanged MODIFIED: %+v", m)
	case <-time.After(500 * time.Millisecond):
		// good
	}
}

func TestStreamEvents_NodeAdded_EmitsTopologyUpdate(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := src.StreamEvents(ctx, model.StreamEventsOptions{})
	require.NoError(t, err)

	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	_, err = client.CoreV1().Nodes().Create(ctx, n, metav1.CreateOptions{})
	require.NoError(t, err)

	msg := readNextOrTimeout(t, ch, 2*time.Second)
	require.NotNil(t, msg)
	assert.Equal(t, "topology.update", msg.Type)
	payload := decodePayload(t, msg.Payload)
	assert.Equal(t, "node-1", payload["nodeName"])
	assert.Equal(t, "node added", payload["delta"])
}

func TestStreamEvents_NodeDeleted_EmitsTopologyUpdate(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := src.StreamEvents(ctx, model.StreamEventsOptions{})
	require.NoError(t, err)

	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	_, err = client.CoreV1().Nodes().Create(ctx, n, metav1.CreateOptions{})
	require.NoError(t, err)
	// Drain the Added emitted by the create.
	first := readNextOrTimeout(t, ch, 2*time.Second)
	require.NotNil(t, first)
	assert.Equal(t, "topology.update", first.Type)
	assert.Equal(t, "node added", decodePayload(t, first.Payload)["delta"])

	require.NoError(t, client.CoreV1().Nodes().Delete(ctx, "node-1", metav1.DeleteOptions{}))
	next := readNextOrTimeout(t, ch, 2*time.Second)
	require.NotNil(t, next)
	assert.Equal(t, "node removed", decodePayload(t, next.Payload)["delta"])
}

func TestStreamEvents_CtxCancel_ClosesChannel(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := src.StreamEvents(ctx, model.StreamEventsOptions{})
	require.NoError(t, err)
	cancel()

	// Channel must close within a brief window (no goroutine leak).
	select {
	case _, ok := <-ch:
		if ok {
			// Could be an in-flight bookmark; read until close.
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StreamEvents goroutine did not exit within 2s of ctx.Cancel")
	}
}

func TestStreamEvents_CtxAlreadyCanceled_ReturnsError(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.StreamEvents(ctx, model.StreamEventsOptions{})
	assert.Error(t, err)
}

func TestStreamEvents_OwnerRefFallbackWhenLabelMissing(t *testing.T) {
	// Pod without app.kubernetes.io/name but WITH an ownerRef; the
	// payload's "name" should be the owner's name (Deployment).
	pod := mkPod("ns", "pod-1", "node-a", corev1.PodRunning, nil, nil)
	pod.OwnerReferences = []metav1.OwnerReference{
		{Name: "my-deploy", Kind: "Deployment"},
	}
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := src.StreamEvents(ctx, model.StreamEventsOptions{})
	require.NoError(t, err)

	_, err = client.CoreV1().Pods("ns").Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err)

	msg := readNextOrTimeout(t, ch, 2*time.Second)
	require.NotNil(t, msg)
	p := decodePayload(t, msg.Payload)
	assert.Equal(t, "my-deploy", p["name"])
}

// ---- helpers ----

func readNextOrTimeout(t *testing.T, ch <-chan *model.WSMessage, d time.Duration) *model.WSMessage {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(d):
		t.Fatalf("no message within %s", d)
		return nil
	}
}

func decodePayload(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}
