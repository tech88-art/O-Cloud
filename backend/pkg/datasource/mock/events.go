package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// eventsFile is the JSON fixture name read by loadEvents from fixturesPath.
// The set-a-small dataset ships events under a single composite document
// (see configs/mock-data/set-a-small/events.json — the same file that holds
// `clusters`, `nodes`, ... at sibling keys).
const eventsFile = "events.json"

// eventsFixture is the on-disk shape — we read only the `events` key. Other
// fixture sections (clusters, nodes, ...) live in the same file but are
// loaded by their own loaders, not this one.
type eventsFixture struct {
	Events []model.Event `json:"events"`
}

// loadEvents reads eventsFile from fixturesPath at most once per Source.
// Sticky error caching matches the pattern in cluster.go / node.go /
// npu.go — once parsing fails we keep returning the same error rather
// than re-hammering a broken file.
//
// Empty fixturesPath returns an empty slice (lets unit tests construct a
// Source with no disk dependency and treat StreamEvents as "no events to
// replay" rather than an error).
func (s *Source) loadEvents() ([]model.Event, error) {
	s.eventsOnce.Do(func() {
		if s.fixturesPath == "" {
			s.events = nil
			return
		}
		path := filepath.Join(s.fixturesPath, eventsFile)
		raw, err := os.ReadFile(path) //nolint:gosec // path comes from server config, not user input
		if err != nil {
			s.eventsErr = fmt.Errorf("read events fixture %q: %w", path, err)
			return
		}
		var doc eventsFixture
		if err := json.Unmarshal(raw, &doc); err != nil {
			s.eventsErr = fmt.Errorf("parse events fixture %q: %w", path, err)
			return
		}
		s.events = doc.Events
	})
	return s.events, s.eventsErr
}

// StreamEvents implements datasource.Source. It replays the fixture's
// events.json honoring each entry's offset from the FIRST event's timestamp
// (events.json uses ISO datetime per configs/mock-data/schema.json, not
// "+5s" strings; the delta-from-event[0] interpretation is what makes the
// storyline replay at human speed).
//
// Behavior:
//   - ctx cancelled at any point → channel closes; in-flight sleep wakes.
//   - All events drained → channel closes naturally.
//   - opts.FastForward > 0 → every delay is divided by that value (tests
//     compress 180s of storyline into <2s).
//   - Channel buffered to 16 so a slow consumer doesn't block the
//     replayer on every single emit; pressure beyond that backs up
//     into the wall-clock delay budget which is fine for our use case.
//
// The returned WSMessage.Timestamp is the SERVER clock at emit time, not
// the event's declared timestamp — frontends animate against wall-clock.
func (s *Source) StreamEvents(ctx context.Context, opts model.StreamEventsOptions) (<-chan *model.WSMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	events, err := s.loadEvents()
	if err != nil {
		return nil, err
	}

	// Buffered channel — see method comment for rationale.
	const replayBuffer = 16
	out := make(chan *model.WSMessage, replayBuffer)

	// Empty fixture: close immediately so callers don't block.
	if len(events) == 0 {
		close(out)
		return out, nil
	}

	go s.replayEvents(ctx, events, opts, out)
	return out, nil
}

// replayEvents is the producer goroutine for StreamEvents. Extracted so the
// nesting in StreamEvents stays readable and so the timing logic has a
// single home for cancellation handling.
func (s *Source) replayEvents(
	ctx context.Context,
	events []model.Event,
	opts model.StreamEventsOptions,
	out chan<- *model.WSMessage,
) {
	defer close(out)

	// Anchor: real wall-clock at the moment the subscriber connects. The
	// first event's declared timestamp becomes "now" from the client's
	// perspective; later events fire at their delta from event[0].
	t0Wall := time.Now()
	t0Event := events[0].Timestamp

	for _, ev := range events {
		delay := eventDelay(t0Event, ev.Timestamp, opts.FastForward)
		target := t0Wall.Add(delay)
		if err := sleepUntil(ctx, target); err != nil {
			// ctx cancelled — stop draining. Channel close happens via
			// deferred close above.
			return
		}

		msg := &model.WSMessage{
			Type:      ev.Type,
			Timestamp: time.Now(),
			Payload:   ev.Payload,
		}
		select {
		case <-ctx.Done():
			return
		case out <- msg:
		}
	}
}

// eventDelay computes how long after t0Wall the event at evTime should
// fire. Negative deltas (event-out-of-order in the fixture) clamp to 0 so
// the replayer never sleeps backwards. FastForward divides the delay; 0
// means real-time.
func eventDelay(t0Event, evTime time.Time, fastForward float64) time.Duration {
	delta := evTime.Sub(t0Event)
	if delta < 0 {
		delta = 0
	}
	if fastForward > 0 {
		delta = time.Duration(float64(delta) / fastForward)
	}
	return delta
}

// sleepUntil parks the caller until target wall-clock or ctx cancellation,
// whichever first. Returns ctx.Err() on cancellation, nil otherwise.
//
// time.Sleep alone doesn't honor cancellation; this is the cancellable
// equivalent. NewTimer (vs time.After) so we Stop() it on cancel and
// don't leak a goroutine waiting on the unread channel.
func sleepUntil(ctx context.Context, target time.Time) error {
	d := time.Until(target)
	if d <= 0 {
		// Already late — let the producer continue without parking. Still
		// honor ctx so a cancelled stream stops promptly.
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
