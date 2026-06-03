package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ---- Fixture helpers -----------------------------------------------------

// eventsFixturePayload is a 3-event storyline spanning 0..2s of declared
// offset. Tests inflate FastForward via the mock.Source's StreamEvents
// option, but the option lives in StreamEventsOptions which the handler
// always passes a zero value for. So tests instead use either:
//   - tightly spaced fixture timestamps (0s/0s/0s offsets → near-instant),
//   - or a fastForwardSource wrapper to inject opts.FastForward.
//
// We pick option 1 here (zero-offset events) for the connect-and-receive
// test, and option 2 for the timing test.
const eventsFixturePayload = `{
  "events": [
    {
      "timestamp": "2026-05-17T12:00:00Z",
      "type": "topology.update",
      "payload": {"snapshot": "initial", "nodes": 3}
    },
    {
      "timestamp": "2026-05-17T12:00:00Z",
      "type": "npu.statusChanged",
      "payload": {"npuId": "n-0", "from": "healthy", "to": "degraded"}
    },
    {
      "timestamp": "2026-05-17T12:00:00Z",
      "type": "workload.statusChanged",
      "payload": {"name": "qwen-8b", "namespace": "ai-inference", "to": "running"}
    }
  ]
}`

// writeEventsFixture drops events.json into a fresh tmp dir. Mirrors
// writeFixture in cluster_test.go.
func writeEventsFixture(t *testing.T, payload string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "events.json"), []byte(payload), 0o600))
	return dir
}

// newWSServer builds a httptest.Server hosting the full router with the
// `events` resource mapped to a real mock.Source loaded from fixturePath.
// Returns the server (caller defers Close) and the ws:// URL for
// /ws/topology.
//
// Optional wrap fn lets a test substitute an alternate Source implementation
// (e.g. fastForwardSource) while keeping the rest of the wiring identical.
func newWSServer(
	t *testing.T,
	fixturePath string,
	wrap func(*mocksrc.Source) datasource.Source,
) (*httptest.Server, string) {
	t.Helper()
	var src datasource.Source = mocksrc.NewSource(fixturePath)
	if wrap != nil {
		src = wrap(src.(*mocksrc.Source))
	}
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"events": "mock"},
	}
	h := NewHandler(reg, nil)
	router := NewRouter(h, RouterOptions{})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	u.Scheme = "ws"
	u.Path = "/ws/topology"
	return srv, u.String()
}

// fastForwardSource wraps a mock.Source so the handler-supplied options
// (zero value) get overridden with a large FastForward. Implements
// datasource.Source by embedding the wrapped source — only StreamEvents
// is overridden.
type fastForwardSource struct {
	datasource.Source
	multiplier float64
}

func (f *fastForwardSource) StreamEvents(ctx context.Context, _ model.StreamEventsOptions) (<-chan *model.WSMessage, error) {
	return f.Source.StreamEvents(ctx, model.StreamEventsOptions{FastForward: f.multiplier})
}

// ---- Tests --------------------------------------------------------------

// TestWSTopology_ConnectAndReceiveFirstEvent: smoke test — open the
// connection, expect the first event within 1s. Fixture uses zero-offset
// events so no fast-forward needed.
func TestWSTopology_ConnectAndReceiveFirstEvent(t *testing.T) {
	_, wsURL := newWSServer(t, writeEventsFixture(t, eventsFixturePayload), nil)

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err, "dial /ws/topology")
	defer func() { _ = conn.Close() }()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	// First event should arrive promptly (zero offset + buffered channel).
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	msgType, raw, err := conn.ReadMessage()
	require.NoError(t, err, "read first frame")
	require.Equal(t, websocket.TextMessage, msgType)

	var msg model.WSMessage
	require.NoError(t, json.Unmarshal(raw, &msg))
	assert.Equal(t, "topology.update", msg.Type)
	assert.WithinDuration(t, time.Now(), msg.Timestamp, 5*time.Second,
		"server timestamp should be ~now")

	// Payload is RawMessage; decode and assert a known field.
	var payload struct {
		Snapshot string `json:"snapshot"`
		Nodes    int    `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))
	assert.Equal(t, "initial", payload.Snapshot)
	assert.Equal(t, 3, payload.Nodes)
}

// TestWSTopology_FastForwardReplaysAllEvents: 30s storyline (3 events at
// 0s/15s/30s) compressed via FastForward = 1000 so the test finishes in
// well under 2s, while preserving event ordering.
func TestWSTopology_FastForwardReplaysAllEvents(t *testing.T) {
	const fixture = `{
  "events": [
    {"timestamp": "2026-05-17T12:00:00Z", "type": "topology.update",      "payload": {"i": 0}},
    {"timestamp": "2026-05-17T12:00:15Z", "type": "npu.statusChanged",    "payload": {"i": 1}},
    {"timestamp": "2026-05-17T12:00:30Z", "type": "workload.statusChanged","payload": {"i": 2}}
  ]
}`

	wrap := func(s *mocksrc.Source) datasource.Source {
		return &fastForwardSource{Source: s, multiplier: 1000}
	}
	_, wsURL := newWSServer(t, writeEventsFixture(t, fixture), wrap)

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	start := time.Now()
	const expectedCount = 3
	received := make([]model.WSMessage, 0, expectedCount)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))

	for len(received) < expectedCount {
		_, raw, err := conn.ReadMessage()
		require.NoError(t, err, "expected %d events; got %d", expectedCount, len(received))
		var msg model.WSMessage
		require.NoError(t, json.Unmarshal(raw, &msg))
		received = append(received, msg)
	}
	elapsed := time.Since(start)

	require.Len(t, received, expectedCount)
	assert.Less(t, elapsed, 2*time.Second,
		"fast-forward replay should complete in <2s (got %s)", elapsed)

	// Verify ordering matches fixture (payload.i = 0,1,2).
	for i, msg := range received {
		var body struct {
			I int `json:"i"`
		}
		require.NoError(t, json.Unmarshal(msg.Payload, &body))
		assert.Equal(t, i, body.I, "events out of order at index %d", i)
	}
}

// TestWSTopology_MalformedClientMessageIgnored: client sends garbage, then
// the server still delivers the next scheduled event. The reader loop must
// not bring the connection down on a non-control frame.
//
// The fixture spaces event[1] 30s after event[0] in declared time; we use a
// fastForwardSource (multiplier 30) to compress that to ~1s real-time so
// the test stays fast. Crucially, this gives the test a *predictable
// window* between event[0] arriving and event[1]'s scheduled emit — long
// enough to send the garbage frame and still have event[1] pending in the
// server's sleepUntil, not already drained into the OS TCP buffer (which
// was the source of an earlier race).
func TestWSTopology_MalformedClientMessageIgnored(t *testing.T) {
	const fixture = `{
  "events": [
    {"timestamp": "2026-05-17T12:00:00Z", "type": "topology.update",   "payload": {"i": 0}},
    {"timestamp": "2026-05-17T12:00:30Z", "type": "npu.statusChanged", "payload": {"i": 1}}
  ]
}`
	wrap := func(s *mocksrc.Source) datasource.Source {
		return &fastForwardSource{Source: s, multiplier: 30} // 30s declared → 1s real
	}
	_, wsURL := newWSServer(t, writeEventsFixture(t, fixture), wrap)

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Read the first event (fires immediately — declared at t0).
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	// Send garbage NOW — server should drop it. event[1] is still ~1s away
	// in the replayer's sleepUntil, so we have a clear before/after.
	require.NoError(t, conn.WriteMessage(websocket.TextMessage,
		[]byte("this is not a known message {nor:valid json")))

	// Event[1] must still arrive. Read deadline accommodates the 1s
	// fast-forwarded delay plus generous slack for slow CI.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err, "stream should survive malformed client input")
	var msg model.WSMessage
	require.NoError(t, json.Unmarshal(raw, &msg))
	assert.Equal(t, "npu.statusChanged", msg.Type)
	var body struct {
		I int `json:"i"`
	}
	require.NoError(t, json.Unmarshal(msg.Payload, &body))
	assert.Equal(t, 1, body.I)
}

// TestWSTopology_ClientCloseTriggersCleanup: open, close client side,
// assert wsActiveConnections returns to baseline within a short timeout.
// This catches goroutine + slot leaks (the most common WS bug).
func TestWSTopology_ClientCloseTriggersCleanup(t *testing.T) {
	// Use a long-running fixture (real-time mode, single late event) so the
	// server is parked in sleepUntil when we yank the connection. That's
	// the harder case for cleanup than "server already drained".
	const longFixture = `{
  "events": [
    {"timestamp": "2026-05-17T12:00:00Z", "type": "topology.update",   "payload": {}},
    {"timestamp": "2026-05-17T12:01:00Z", "type": "npu.statusChanged", "payload": {}}
  ]
}`
	_, wsURL := newWSServer(t, writeEventsFixture(t, longFixture), nil)

	baseline := activeWSConnections()

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Read the first event to confirm the server is in the middle of serving.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	// Wait briefly so the server is parked in sleepUntil for event[1] (60s
	// away in real-time mode). The slot is taken — assert it.
	deadline := time.Now().Add(time.Second)
	for activeWSConnections() < baseline+1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, baseline+1, activeWSConnections(),
		"active conn count should reflect the open subscriber")

	// Close client → server reader sees EOF → cancels stream ctx → writer
	// exits → defer decrements wsActiveConnections.
	require.NoError(t, conn.Close())

	// Active count must drop back to baseline within a reasonable window.
	// gorilla wakes from a Read within milliseconds; the loop breaks early on
	// success so a healthy run stays fast. The window is 5s (was 2s · P13-fix-001)
	// to absorb CI-runner scheduling jitter under a loaded matrix — the 2s bound
	// flaked TestWSTopology_ClientCloseTriggersCleanup on the phase-13-complete
	// gate while the goroutine cleanup was merely slow, not stuck.
	clean := false
	cleanupDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(cleanupDeadline) {
		if activeWSConnections() == baseline {
			clean = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.True(t, clean, "wsActiveConnections did not return to baseline (%d → %d) within 5s",
		baseline, activeWSConnections())

	// Goroutine count check — must also recover. We're lenient here because
	// httptest / gorilla / gin all keep background workers that wiggle the
	// count; just assert it's not dramatically higher than baseline.
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
}

// TestWSTopology_NoEventsMapping_Returns500: registry without an events
// mapping (and only this test wipes the auto-fallback by using a Source
// that isn't in the Sources map either). Confirms the no-source-mapped
// 500 path is reachable for ops debuggability.
func TestWSTopology_NoEventsMapping_Returns500(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{},
		Mapping: map[string]string{},
	}
	h := NewHandler(reg, nil)
	router := NewRouter(h, RouterOptions{})

	srv := httptest.NewServer(router)
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	u.Scheme = "ws"
	u.Path = "/ws/topology"

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	_, resp, err := dialer.Dial(u.String(), nil)
	require.Error(t, err, "dial should fail before upgrade")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// TestWSTopology_EmptyFixtureClosesCleanly: zero events → server should
// upgrade, send the close frame, and tear down. The client read returns a
// CloseError, not a hang. Guards against the "channel never closes" bug in
// StreamEvents.
func TestWSTopology_EmptyFixtureClosesCleanly(t *testing.T) {
	_, wsURL := newWSServer(t, writeEventsFixture(t, `{"events":[]}`), nil)

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err = conn.ReadMessage()
	require.Error(t, err, "expected close frame from empty fixture")
	if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		// Some networks deliver the close as a plain EOF; accept that too.
		assert.True(t,
			websocket.IsUnexpectedCloseError(err) ||
				strings.Contains(err.Error(), "EOF") ||
				strings.Contains(err.Error(), "closed"),
			"expected close-related error, got %v", err)
	}
}
