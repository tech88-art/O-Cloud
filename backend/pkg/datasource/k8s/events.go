package k8s

import (
	"context"
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// StreamEvents subscribes to apiserver watches for Pods + Nodes and
// translates incoming events into model.WSMessage envelopes matching
// the OpenAPI contract enum:
//
//   - Pod Added            → workload.created
//   - Pod Deleted          → workload.deleted
//   - Pod Modified         → workload.statusChanged (when phase changed)
//   - Node Added/Modified  → topology.update (delta describes the change)
//   - Node Deleted         → topology.update with delta="node removed"
//
// Design notes:
//
//   - We use direct corev1 Watch calls rather than informer factories.
//     For the Phase 2 demo (few subscribers, small clusters) the
//     simpler direct-watch path is enough; Phase 9+ scale can migrate
//     to a shared informer factory + watch multiplexing.
//
//   - Each StreamEvents call gets its own pair of watches. Channel
//     ownership: the returned channel is OWNED by the goroutine in
//     this function; closing happens exactly once on ctx cancellation
//     or upstream watch closure. No goroutine leaks per the AC.
//
//   - opts.FastForward is ignored — the K8s watch stream is naturally
//     real-time, no replay timing to compress. The knob exists for the
//     mock source's events.json replayer; we accept (and silently drop)
//     it here so the WS handler doesn't need source-aware branching.
//
//   - WSMessage.payload is json.RawMessage. We marshal a small per-
//     event object and stuff the bytes in; marshal failure logs and
//     skips the frame rather than tearing down the whole stream.
//
//   - Initial-state replay: apiserver Watch with ResourceVersion=""
//     starts at the current revision; Phase 2 doesn't replay history
//     (the frontend uses the REST endpoints for initial state, then
//     subscribes to the WS for deltas).
func (s *Source) StreamEvents(ctx context.Context, _ model.StreamEventsOptions) (<-chan *model.WSMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	podWatcher, err := s.client.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errFromAPIServer("watch pods", err)
	}
	nodeWatcher, err := s.client.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{})
	if err != nil {
		podWatcher.Stop()
		return nil, errFromAPIServer("watch nodes", err)
	}

	// Buffer of 32 absorbs short consumer stalls without dropping
	// frames. Slow consumers that fall further behind will block
	// the producer goroutine — that's the chosen back-pressure
	// strategy (vs dropping events silently which produces UI drift).
	out := make(chan *model.WSMessage, 32)
	go pumpEvents(ctx, podWatcher, nodeWatcher, out)
	return out, nil
}

// pumpEvents drains both watchers + emits translated WSMessages until
// ctx done OR both watchers closed. Channel is closed exactly once.
func pumpEvents(ctx context.Context, podW, nodeW watch.Interface, out chan<- *model.WSMessage) {
	defer close(out)
	defer podW.Stop()
	defer nodeW.Stop()

	// Track last-seen pod phase per (ns, name) so phase-unchanged
	// MODIFIED events don't generate noise. We DO emit on any
	// modification today (the frontend invalidates the topology
	// react-query cache); future filtering by "interesting" delta
	// goes here.
	podPhase := map[string]corev1.PodPhase{}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-podW.ResultChan():
			if !ok {
				// Upstream watch closed; close our channel by exiting.
				return
			}
			msg := translatePodEvent(ev, podPhase)
			if msg == nil {
				continue
			}
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		case ev, ok := <-nodeW.ResultChan():
			if !ok {
				return
			}
			msg := translateNodeEvent(ev)
			if msg == nil {
				continue
			}
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}
}

// translatePodEvent maps a watch.Event on Pods to a WSMessage.
// Returns nil when the event is uninteresting (bookmark, error,
// untyped object, or a MODIFIED with unchanged phase).
func translatePodEvent(ev watch.Event, lastPhase map[string]corev1.PodPhase) *model.WSMessage {
	pod, ok := ev.Object.(*corev1.Pod)
	if !ok {
		return nil
	}
	key := pod.Namespace + "/" + pod.Name
	switch ev.Type {
	case watch.Added:
		lastPhase[key] = pod.Status.Phase
		return wsMessageFromPod("workload.created", pod)
	case watch.Deleted:
		delete(lastPhase, key)
		return wsMessageFromPod("workload.deleted", pod)
	case watch.Modified:
		prev, seen := lastPhase[key]
		lastPhase[key] = pod.Status.Phase
		if !seen || prev != pod.Status.Phase {
			return wsMessageFromPod("workload.statusChanged", pod)
		}
		return nil
	default:
		// Bookmark / Error / unknown — drop silently. The Error frame
		// surfaces a transport-level problem (auth expired, etc.) that
		// the writer loop would just retry on its own.
		return nil
	}
}

// translateNodeEvent maps a watch.Event on Nodes to a WSMessage.
// All node changes surface as topology.update with a small delta
// describing what happened.
func translateNodeEvent(ev watch.Event) *model.WSMessage {
	node, ok := ev.Object.(*corev1.Node)
	if !ok {
		return nil
	}
	switch ev.Type {
	case watch.Added, watch.Modified, watch.Deleted:
		return wsMessageFromNode(ev.Type, node)
	default:
		return nil
	}
}

// wsMessageFromPod packs the workload.* envelope. payload carries
// the namespace+name+phase+(workload owner identity inferred from
// labels) — minimal but enough for the frontend to dispatch an
// invalidation. Marshal failure → nil (caller drops the frame).
func wsMessageFromPod(t string, pod *corev1.Pod) *model.WSMessage {
	owner := pod.Labels["app.kubernetes.io/name"]
	if owner == "" {
		// Fall back to ownerReferences if present; surfaces the
		// Deployment/StatefulSet/Job name for the typical case.
		if len(pod.OwnerReferences) > 0 {
			owner = pod.OwnerReferences[0].Name
		}
	}
	payload, err := json.Marshal(map[string]any{
		"namespace": pod.Namespace,
		"name":      owner,
		"podName":   pod.Name,
		"nodeName":  pod.Spec.NodeName,
		"phase":     string(pod.Status.Phase),
	})
	if err != nil {
		return nil
	}
	return &model.WSMessage{
		Type:      t,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}

// wsMessageFromNode packs the topology.update envelope. delta hints
// at the cause (added / modified / removed) so the frontend can log
// it for ops; the load-bearing field is the timestamp that drives
// react-query invalidation.
func wsMessageFromNode(eventType watch.EventType, node *corev1.Node) *model.WSMessage {
	deltaHint := ""
	switch eventType {
	case watch.Added:
		deltaHint = "node added"
	case watch.Modified:
		deltaHint = "node modified"
	case watch.Deleted:
		deltaHint = "node removed"
	default:
		deltaHint = string(eventType)
	}
	payload, err := json.Marshal(map[string]any{
		"nodeName": node.Name,
		"delta":    deltaHint,
	})
	if err != nil {
		return nil
	}
	return &model.WSMessage{
		Type:      "topology.update",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}
