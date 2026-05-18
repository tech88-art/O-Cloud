package k8s

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// GetWorkloadLogs returns a bounded tail of log lines from the first
// pod of the named workload. Phase 2 implementation: discover the
// workload (via GetWorkloadDetail's selector-driven path), pick its
// first pod, open the kubelet log endpoint with TailLines /
// SinceTime, parse the raw byte stream line-by-line into model.LogLine.
//
// Container choice: if opts.Container is set we honour it; absent →
// the pod's first container (matches what `kubectl logs <pod>` does).
//
// Multi-pod workloads (replicas > 1) only surface the first pod's
// logs here — this matches mock.Source's behaviour and the Logs page
// contract. A future "aggregate logs across pods" mode would need a
// schema decision (interleave vs concat) and is out of scope for
// Phase 2.
//
// HasMore is always false in Phase 2 — the kubelet endpoint is
// stateless and there's no cursor to thread; the page caps at
// opts.Tail (default 200) and that's the round-trip.
func (s *Source) GetWorkloadLogs(ctx context.Context, namespace, name string, opts model.LogOptions) (*model.LogPage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if namespace == "" || name == "" {
		return nil, ErrResourceNotFound
	}

	pod, container, err := s.resolveWorkloadPod(ctx, namespace, name, opts.Container)
	if err != nil {
		return nil, err
	}

	tail := opts.Tail
	if tail <= 0 {
		tail = 200
	}
	tail64 := int64(tail)

	podOpts := &corev1.PodLogOptions{
		Container: container,
		TailLines: &tail64,
		Timestamps: true,
	}
	if opts.Since != "" {
		if t, err := time.Parse(time.RFC3339, opts.Since); err == nil {
			ts := metav1.NewTime(t)
			podOpts.SinceTime = &ts
		}
		// Silently drop malformed since — the page contract is "show
		// SOMETHING reasonable", not "400 on bad ISO".
	}

	req := s.client.CoreV1().Pods(namespace).GetLogs(pod, podOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrResourceNotFound
		}
		return nil, errFromAPIServer("open pod logs", err)
	}
	defer func() { _ = stream.Close() }()

	lines, err := readLogLines(stream, container)
	if err != nil {
		return nil, errFromAPIServer("read pod logs", err)
	}
	return &model.LogPage{
		Lines:   lines,
		HasMore: false,
	}, nil
}

// StreamWorkloadLogs opens a follow stream against the first pod's
// chosen container and ships LogLine entries on the returned channel.
// Channel closes when ctx is cancelled or the kubelet upstream closes
// (Pod restart / container exit). Same pod-selection semantics as
// GetWorkloadLogs.
func (s *Source) StreamWorkloadLogs(ctx context.Context, namespace, name string, opts model.LogStreamOptions) (<-chan *model.LogLine, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if namespace == "" || name == "" {
		return nil, ErrResourceNotFound
	}

	pod, container, err := s.resolveWorkloadPod(ctx, namespace, name, opts.Container)
	if err != nil {
		return nil, err
	}

	podOpts := &corev1.PodLogOptions{
		Container:  container,
		Follow:     true,
		Timestamps: true,
	}
	if opts.Since != "" {
		if t, err := time.Parse(time.RFC3339, opts.Since); err == nil {
			ts := metav1.NewTime(t)
			podOpts.SinceTime = &ts
		}
	}

	stream, err := s.client.CoreV1().Pods(namespace).GetLogs(pod, podOpts).Stream(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrResourceNotFound
		}
		return nil, errFromAPIServer("open pod log stream", err)
	}

	out := make(chan *model.LogLine, 32)
	go pumpLogStream(ctx, stream, container, out)
	return out, nil
}

// resolveWorkloadPod returns (podName, containerName) for the named
// workload. Containers default to the pod spec's first container;
// callers can override via opts.Container.
//
// Tries Deployment → StatefulSet → Job in turn — same fallback chain
// as GetWorkloadDetail, but it returns early on the first match.
//
// Pod selection: lists pods with the workload's label selector and
// picks the lex-smallest pod name so two consecutive calls hit the
// same pod even when the cluster scales / restarts.
func (s *Source) resolveWorkloadPod(ctx context.Context, namespace, name, containerOverride string) (string, string, error) {
	selector, err := s.workloadSelector(ctx, namespace, name)
	if err != nil {
		return "", "", err
	}
	pods, err := s.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return "", "", errFromAPIServer("list workload pods for logs", err)
	}
	if len(pods.Items) == 0 {
		return "", "", ErrResourceNotFound
	}

	// Pick lex-smallest pod name for stable selection.
	chosen := &pods.Items[0]
	for i := range pods.Items {
		if pods.Items[i].Name < chosen.Name {
			chosen = &pods.Items[i]
		}
	}

	container := containerOverride
	if container == "" {
		if len(chosen.Spec.Containers) == 0 {
			return "", "", errors.New("k8s logs: pod has zero containers")
		}
		container = chosen.Spec.Containers[0].Name
	} else {
		// Validate the override against the pod's container list so
		// requesting a non-existent container gets a clean 404 instead
		// of a kubelet error mid-stream.
		found := false
		for _, c := range chosen.Spec.Containers {
			if c.Name == container {
				found = true
				break
			}
		}
		if !found {
			return "", "", ErrResourceNotFound
		}
	}
	return chosen.Name, container, nil
}

// workloadSelector resolves the workload's pod label selector by
// trying Deployment → StatefulSet → Job in turn. Returns the
// comma-separated `k=v,k=v` selector form ready for ListOptions.
func (s *Source) workloadSelector(ctx context.Context, namespace, name string) (string, error) {
	if d, err := s.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		return labels(d.Spec.Selector.MatchLabels).asLabelSelector(), nil
	} else if !apierrors.IsNotFound(err) {
		return "", errFromAPIServer("get deployment for log selector", err)
	}
	if ss, err := s.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		return labels(ss.Spec.Selector.MatchLabels).asLabelSelector(), nil
	} else if !apierrors.IsNotFound(err) {
		return "", errFromAPIServer("get statefulset for log selector", err)
	}
	if j, err := s.client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		return labels(j.Spec.Selector.MatchLabels).asLabelSelector(), nil
	} else if !apierrors.IsNotFound(err) {
		return "", errFromAPIServer("get job for log selector", err)
	}
	return "", ErrResourceNotFound
}

// readLogLines drains the kubelet log byte stream into structured
// model.LogLine entries. Each line is expected to start with an
// RFC3339Nano timestamp (kubelet `--timestamps=true`) followed by a
// space and the raw container message; the parser is tolerant of
// missing timestamps for pre-1.22 kubelets / non-timestamped logs.
func readLogLines(r io.Reader, container string) ([]model.LogLine, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB cap per line
	out := []model.LogLine{}
	for scanner.Scan() {
		out = append(out, parseLogLine(scanner.Text(), container))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// parseLogLine splits the kubelet log format into a model.LogLine.
// Expected shape: "2026-05-18T09:36:45.218Z message body".
// Falls back to a zero-timestamp + raw-message line when the prefix
// doesn't look like a date.
func parseLogLine(raw, container string) model.LogLine {
	out := model.LogLine{Container: container, Message: raw}
	sp := strings.IndexByte(raw, ' ')
	if sp <= 0 {
		return out
	}
	tsStr := raw[:sp]
	body := raw[sp+1:]
	t, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		return out
	}
	out.Timestamp = t.UTC()
	out.Message = body
	// Heuristic level extraction — most container logs begin with
	// `[level]` or `LEVEL:` after the timestamp. Cheap to scan; helps
	// the Logs page colour-code without server-side semantics.
	out.Level = extractLevelHint(body)
	return out
}

// extractLevelHint inspects the first ~16 bytes for common level
// prefixes. Returns "" when nothing recognised — the LogViewer
// falls back to its default colour.
func extractLevelHint(body string) string {
	if body == "" {
		return ""
	}
	head := body
	if len(head) > 24 {
		head = head[:24]
	}
	upper := strings.ToUpper(head)
	for _, lvl := range []string{"FATAL", "ERROR", "WARN", "INFO", "DEBUG", "TRACE"} {
		if strings.Contains(upper, lvl) {
			return lvl
		}
	}
	return ""
}

// pumpLogStream is the background reader for StreamWorkloadLogs. It
// drains the kubelet follow stream until ctx done / stream EOF /
// error, then closes the output channel.
func pumpLogStream(ctx context.Context, r io.ReadCloser, container string, out chan<- *model.LogLine) {
	defer close(out)
	defer func() { _ = r.Close() }()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := parseLogLine(scanner.Text(), container)
		select {
		case out <- &line:
		case <-ctx.Done():
			return
		}
	}
	// Scanner errors are non-fatal: the channel close signals the
	// consumer; the logger pkg (when wired in P2-T-006) can surface
	// the cause via h.Logger. For now we drop the error since this
	// goroutine has no logger reference.
}
