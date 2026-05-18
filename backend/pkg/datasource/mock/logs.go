// Package mock — workload logs (P1-T-301).
//
// GetWorkloadLogs synthesizes a deterministic tail of LogLine entries for the
// requested workload. StreamWorkloadLogs spins a producer goroutine that emits
// a fresh LogLine on a steady cadence until the caller cancels the context.
//
// Why procedural instead of fixture-driven:
//   - The demo Logs page doesn't need scripted content — it needs a believable
//     "looks like a running workload" stream. A 30-line fixture would loop
//     after half a minute and break the illusion.
//   - Deterministic seed keeps the REST tail reproducible for tests; the WS
//     stream is time-driven so its content varies but is still bounded by the
//     same message bank.
//
// The line generator is workload-state aware: a "failed" workload mixes in
// stack-trace-shaped ERROR lines; a "running" workload is mostly INFO with
// occasional throughput / latency metrics. Container names come from the
// workload's first pod's containers[] when present, else fall back to a
// synthetic "main" tag so the wire shape always carries a container field.
package mock

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// logTickInterval is how often StreamWorkloadLogs emits a new line. 1s feels
// alive without spamming the WebSocket; PHASE-2 will pull from real source so
// this constant becomes irrelevant. Exported-ish via test override.
const logTickInterval = 1 * time.Second

// logTailDefault is what GetWorkloadLogs returns when LogOptions.Tail <= 0.
// Matches the OpenAPI default of 200.
const logTailDefault = 200

// logTailMax bounds how many lines a single REST call can return. 2000 is
// the same ceiling kubectl logs uses for --tail before it warns; we adopt it
// here so demo sessions don't accidentally pull megabytes per request.
const logTailMax = 2000

// findWorkload is the namespace+name lookup helper shared by REST + WS log
// methods. Mirrors GetWorkloadDetail's error semantics — returns
// ErrWorkloadNotFound (404) when no fixture matches, surface load errors as-is.
//
// Returns a pointer into the cache; callers MUST treat the result as read-only
// (no mutation) since the cache is shared across all source consumers.
func (s *Source) findWorkload(namespace, name string) (*model.WorkloadDetail, error) {
	all, err := s.loadWorkloads()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Namespace == namespace && all[i].Name == name {
			return &all[i], nil
		}
	}
	return nil, ErrWorkloadNotFound
}

// GetWorkloadLogs synthesizes `tail` log lines for the requested workload.
// Lines are ordered oldest-first (chronologically ascending) with the newest
// line timestamped at call-time. Container filter, when set, drops lines
// from other containers BEFORE the tail count is applied.
//
// hasMore + nextCursor stay false / nil in Phase 1 — the mock is finite, so
// the contract's pagination signal would be misleading; PHASE-2 with k8s
// source will wire real cursors.
func (s *Source) GetWorkloadLogs(ctx context.Context, namespace, name string, opts model.LogOptions) (*model.LogPage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wl, err := s.findWorkload(namespace, name)
	if err != nil {
		return nil, err
	}

	tail := opts.Tail
	if tail <= 0 {
		tail = logTailDefault
	}
	if tail > logTailMax {
		tail = logTailMax
	}

	gen := newLogGenerator(wl, opts.Container)
	now := time.Now().UTC()

	// Walk backward from `now` so the most recent line lands at index len-1.
	// Step is the stream cadence; using the same interval keeps REST + WS
	// timestamps visually consistent.
	lines := make([]model.LogLine, 0, tail)
	for i := tail - 1; i >= 0; i-- {
		ts := now.Add(-time.Duration(i) * logTickInterval)
		lines = append(lines, gen.lineAt(ts))
	}

	return &model.LogPage{
		Lines:      lines,
		HasMore:    false,
		NextCursor: nil,
	}, nil
}

// StreamWorkloadLogs returns a channel that emits a fresh LogLine every
// logTickInterval until the caller cancels ctx. Channel closes on ctx done;
// no error after the initial 404 path.
//
// The producer uses its own seeded RNG so concurrent subscribers don't see
// identical content (each Stream call advances independently). The channel
// is buffered (size 16) so a momentarily-slow consumer doesn't stall the
// ticker — when the buffer fills the producer drops the oldest pending
// line (best-effort delivery; logs are advisory).
func (s *Source) StreamWorkloadLogs(ctx context.Context, namespace, name string, opts model.LogStreamOptions) (<-chan *model.LogLine, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wl, err := s.findWorkload(namespace, name)
	if err != nil {
		return nil, err
	}

	gen := newLogGenerator(wl, opts.Container)
	out := make(chan *model.LogLine, 16)

	go func() {
		defer close(out)
		ticker := time.NewTicker(logTickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				line := gen.lineAt(t.UTC())
				select {
				case out <- &line:
				default:
					// Buffer full — drop the oldest by popping then pushing.
					// Best-effort: a stuck consumer must reconnect.
					select {
					case <-out:
					default:
					}
					select {
					case out <- &line:
					default:
					}
				}
			}
		}
	}()

	return out, nil
}

// logGenerator owns the per-stream state: the workload context, the candidate
// container list (filtered if opts.Container is set), and a seeded RNG. The
// generator is goroutine-safe-by-construction since each Stream call gets its
// own instance; GetWorkloadLogs uses a fresh one per call.
type logGenerator struct {
	rng        *rand.Rand
	workload   string
	namespace  string
	status     string
	containers []string
	// counter advances per emitted line — feeds bracket index variation in
	// pseudo-stack-trace ERROR lines so failed-workload tails read like a
	// repeating tail of a real log, not a stamped-pattern fake.
	counter uint64
}

// newLogGenerator builds the per-call generator. The seed is workload-stable
// (so GetWorkloadLogs(ns,name) is reproducible across requests within the
// same second) but timestamp-perturbed so two consecutive REST calls in the
// same second still produce slightly different content.
func newLogGenerator(wl *model.WorkloadDetail, containerFilter string) *logGenerator {
	seed := stableSeed(wl.Namespace + "/" + wl.Name)
	containers := collectContainers(wl, containerFilter)
	if len(containers) == 0 {
		// Fixture has no container info → synthesize a default tag so the
		// wire shape always carries a non-empty `container` field.
		containers = []string{"main"}
	}
	return &logGenerator{
		rng:        rand.New(rand.NewSource(seed)), //nolint:gosec // not security-sensitive
		workload:   wl.Name,
		namespace:  wl.Namespace,
		status:     wl.Status,
		containers: containers,
	}
}

// stableSeed produces a 64-bit seed from the workload key. FNV-1a is cheap and
// deterministic; we don't need cryptographic quality here.
func stableSeed(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64()) //nolint:gosec // intentional cast, not a secret
}

// collectContainers reads the workload's first pod's containers as the
// candidate set, optionally filtered to a single container name. Workloads
// with no pods or empty containers[] return nil — caller substitutes "main".
func collectContainers(wl *model.WorkloadDetail, filter string) []string {
	if len(wl.Pods) == 0 {
		return nil
	}
	pod := wl.Pods[0]
	out := make([]string, 0, len(pod.Containers))
	for _, c := range pod.Containers {
		if c.Name == "" {
			continue
		}
		if filter != "" && c.Name != filter {
			continue
		}
		out = append(out, c.Name)
	}
	return out
}

// lineAt renders one LogLine at the supplied timestamp. Picks a container
// round-robin via the counter, picks a level/message bank by workload
// status, and increments the per-generator counter.
func (g *logGenerator) lineAt(ts time.Time) model.LogLine {
	container := g.containers[int(g.counter%uint64(len(g.containers)))]
	level, message := g.pickContent()
	g.counter++
	return model.LogLine{
		Timestamp: ts,
		Level:     level,
		Container: container,
		Message:   message,
	}
}

// pickContent chooses a (level, message) pair weighted by workload status.
// Failed workloads see ~30% ERROR + 20% WARN + 50% INFO; running workloads
// see ~5% WARN + 95% INFO. Other statuses default to mostly-INFO.
func (g *logGenerator) pickContent() (string, string) {
	roll := g.rng.Intn(100)
	switch g.status {
	case "failed", "Failed":
		if roll < 30 {
			return "ERROR", g.pickError()
		}
		if roll < 50 {
			return "WARN", g.pickWarn()
		}
		return "INFO", g.pickInfo()
	case "running", "Running":
		if roll < 5 {
			return "WARN", g.pickWarn()
		}
		return "INFO", g.pickInfo()
	default:
		if roll < 10 {
			return "WARN", g.pickWarn()
		}
		return "INFO", g.pickInfo()
	}
}

// Message banks — small but varied enough to read as live output. Each
// entry is a function pointer so the arg count is encapsulated next to the
// template (using fmt-style %d strings with mismatched arg counts would
// inject ugly `%!(EXTRA int=...)` into the wire payload). The closure
// signature is uniform `(rng) -> string` so the picker is a single line.

type msgBuilder func(rng *rand.Rand) string

var infoMessages = []msgBuilder{
	func(r *rand.Rand) string {
		return fmt.Sprintf("request handled in %dms (route=/v1/chat/completions)", r.Intn(900)+50)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("prefill batch=%d kv_cache_pages_used=%d", r.Intn(16)+1, r.Intn(900)+50)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("decode tokens_generated=%d throughput=%d tok/s", r.Intn(900)+50, r.Intn(900)+50)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("npu kernel launched stream=%d shape=[1, %d, 4096]", r.Intn(8), r.Intn(900)+50)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("hccs allreduce ring=%d size=%dMB latency=%dus", r.Intn(8), r.Intn(900)+50, r.Intn(900)+50)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("scheduler accepted request id=req-%d", r.Intn(9000)+1000)
	},
	func(_ *rand.Rand) string {
		return "warmup pass complete, switching to steady-state"
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("gc reclaimed %dMB (heap %dMB)", r.Intn(900)+50, r.Intn(900)+50)
	},
}

var warnMessages = []msgBuilder{
	func(r *rand.Rand) string {
		return fmt.Sprintf("slow tokenizer pass took %dms — falling back to fast path", r.Intn(900)+50)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("kv cache eviction triggered (pressure %d%%)", r.Intn(40)+60)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("npu temperature elevated: %d°C (threshold 85°C)", r.Intn(15)+80)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("queue depth %d exceeds soft cap, backpressure engaged", r.Intn(500)+100)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("checkpoint write delayed by %dms (disk i/o saturated)", r.Intn(900)+50)
	},
}

var errorMessages = []msgBuilder{
	func(r *rand.Rand) string {
		return fmt.Sprintf("npu kernel launch failed: invalid memory access at 0x%x", r.Intn(0xffff)+0x1000)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("hccs link %d dropped — falling back to dual-rail", r.Intn(8))
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("OOM in container, evicting prefill batch (used %dMB)", r.Intn(900)+50)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("request timeout after %dms — client cancelled", r.Intn(900)+5000)
	},
	func(r *rand.Rand) string {
		return fmt.Sprintf("runtime assertion: tensor rank mismatch (expected %d, got %d)", r.Intn(4)+2, r.Intn(4)+2)
	},
}

func (g *logGenerator) pickInfo() string {
	return infoMessages[g.rng.Intn(len(infoMessages))](g.rng)
}

func (g *logGenerator) pickWarn() string {
	return warnMessages[g.rng.Intn(len(warnMessages))](g.rng)
}

func (g *logGenerator) pickError() string {
	return errorMessages[g.rng.Intn(len(errorMessages))](g.rng)
}
