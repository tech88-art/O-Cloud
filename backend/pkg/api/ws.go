package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// resourceEvents is the registry key for WebSocket event streams. Mirrors
// `mapping.events` in config.yaml. Falls back to the topology/clusters
// mapping via eventsSource() when no explicit events mapping exists — same
// pattern as topologySource() in cluster.go.
const resourceEvents = "events"

// WebSocket tuning constants. Match common idiomatic gorilla values; we
// keep them small and documented so PHASE-2 can tune from one place.
const (
	// wsWriteWait bounds how long the server is willing to block writing a
	// single message before declaring the client dead.
	wsWriteWait = 10 * time.Second

	// wsPongWait is the max interval between pongs from the client. If no
	// pong arrives within this window, the connection is closed.
	wsPongWait = 60 * time.Second

	// wsPingPeriod is how often the server pings the client. Must be < pong
	// wait. 30s comes from the task spec ("heartbeat ping every 30s").
	wsPingPeriod = 30 * time.Second

	// wsMaxMessageSize caps inbound message size. We don't accept large
	// payloads from clients on /ws/topology — anything inbound is treated
	// as a control message (and currently ignored).
	wsMaxMessageSize int64 = 4096
)

// wsMaxConnections caps the number of concurrent /ws/topology subscribers
// to avoid a slow-loris style resource exhaustion (each conn parks two
// goroutines + a buffered channel of *WSMessage). 100 is a generous demo
// ceiling; PHASE-2 will derive this from config.
//
//nolint:gochecknoglobals // package-level atomic counter; the natural fit
var wsActiveConnections atomic.Int32

const wsMaxConnections int32 = 100

// wsUpgrader is the shared websocket upgrader for /ws/* endpoints.
//
// PHASE-1: permissive CheckOrigin so the dev frontend (Vite on a different
// port) connects without CORS headaches. PHASE-2 narrows once auth lands.
//
//nolint:gochecknoglobals // intentional package-level upgrader (gorilla idiom)
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WSTopology handles GET /ws/topology — upgrade to WebSocket and stream
// WSMessage envelopes from the events source. See backend/CLAUDE.md §4.7
// for the wire format. Per docs/api-contract.yaml, this endpoint lives
// OUTSIDE /api/v1 (the contract comment block explicitly lists
// `/ws/topology` at the root).
//
// Flow:
//  1. Capacity check; 503 if at cap (before upgrade — easier on clients).
//  2. Upgrade. wsUpgrader.Upgrade returns 4xx/5xx on its own on failure.
//  3. Subscribe to source.StreamEvents → channel of *WSMessage.
//  4. Reader goroutine consumes client messages (mostly control frames),
//     surfaces disconnect/read errors via the connection's deadline.
//  5. Writer loop reads from the events channel, pings every wsPingPeriod,
//     writes each WSMessage as JSON. Exits on ctx done, channel close,
//     client disconnect, or write error.
func (h *Handler) WSTopology(c *gin.Context) {
	// Capacity gate. Atomic so concurrent upgrades don't race past the cap.
	if cur := wsActiveConnections.Load(); cur >= wsMaxConnections {
		h.Logger.Warn("ws topology: connection cap reached",
			zap.Int32("active", cur),
			zap.Int32("max", wsMaxConnections))
		respondError(c, http.StatusServiceUnavailable, CodeInternalError,
			"websocket connection cap reached", nil)
		return
	}

	src := h.eventsSource()
	if src == nil {
		h.Logger.Error("no datasource mapped for events")
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource mapped for events", nil)
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response; just log.
		h.Logger.Warn("ws topology: upgrade failed", zap.Error(err))
		return
	}

	// Increment AFTER successful upgrade so failed upgrades don't burn a
	// slot. Decrement is unconditional in serveWSTopology's defer.
	wsActiveConnections.Add(1)
	h.serveWSTopology(c.Request.Context(), conn, src)
}

// serveWSTopology owns one upgraded connection from cradle to grave. The
// caller has already incremented wsActiveConnections; we decrement on
// return regardless of how we exited.
//
// reqCtx is the *original* HTTP request context — Gin cancels it when the
// HTTP server shuts down, giving us clean shutdown for free. We derive a
// child ctx that we cancel ourselves on disconnect so the StreamEvents
// goroutine doesn't outlive the connection.
func (h *Handler) serveWSTopology(reqCtx context.Context, conn *websocket.Conn, src datasource.Source) {
	defer func() {
		_ = conn.Close()
		wsActiveConnections.Add(-1)
	}()

	streamCtx, cancel := context.WithCancel(reqCtx)
	defer cancel()

	// Configure read side first — needed for ping/pong heartbeat to work.
	conn.SetReadLimit(wsMaxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})

	// Subscribe BEFORE starting the reader so we 500 cleanly if the source
	// errors out (rather than mid-handshake from the client's view).
	events, err := src.StreamEvents(streamCtx, model.StreamEventsOptions{})
	if err != nil {
		h.Logger.Error("ws topology: stream events failed",
			zap.String("source", src.Name()),
			zap.Error(err))
		_ = writeJSON(conn, errorWSMessage("failed to subscribe to events"))
		return
	}

	// Reader goroutine: consume client messages so pong handler fires and
	// disconnect is detected. We don't act on any inbound message in
	// Phase 1 — the contract is server-pushed events only. Cancel the
	// stream ctx on read error so the writer loop unblocks.
	go h.readerLoop(streamCtx, conn, cancel)

	h.writerLoop(streamCtx, conn, events)
}

// readerLoop drains inbound frames. Malformed / unexpected messages are
// ignored (the contract is push-only). The loop exits when the client
// closes or the read deadline expires; either way it cancels the parent
// ctx so the writer side wakes and tears down.
func (h *Handler) readerLoop(_ context.Context, conn *websocket.Conn, cancelStream context.CancelFunc) {
	defer cancelStream()
	for {
		// ReadMessage is the only call that drives the pong handler. We
		// discard the payload — Phase 1 has no inbound contract.
		if _, _, err := conn.ReadMessage(); err != nil {
			if !websocket.IsCloseError(err,
				websocket.CloseNormalClosure,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure) {
				h.Logger.Debug("ws topology: reader exit", zap.Error(err))
			}
			return
		}
		// Malformed messages: ignored on purpose.
	}
}

// writerLoop pumps events to the client and pings on schedule. Exits on
// any of: ctx done, events channel closed, client write error, ping write
// error. Returns are silent — the reader loop logs the underlying cause.
func (h *Handler) writerLoop(ctx context.Context, conn *websocket.Conn, events <-chan *model.WSMessage) {
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Best-effort close frame so well-behaved clients get a clean
			// shutdown rather than an EOF. Errors here are uninteresting —
			// we're tearing down regardless.
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				h.Logger.Debug("ws topology: ping write failed", zap.Error(err))
				return
			}
		case msg, ok := <-events:
			if !ok {
				// Source drained — signal the client and exit. Same close
				// frame as ctx-done path so clients can't tell the
				// difference; both are "stream complete".
				_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := writeJSON(conn, msg); err != nil {
				h.Logger.Debug("ws topology: message write failed", zap.Error(err))
				return
			}
		}
	}
}

// writeJSON marshals + writes a single text frame, honoring write
// deadline. Extracted so error-message + replay paths share encoding.
func writeJSON(conn *websocket.Conn, v interface{}) error {
	_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// errorWSMessage builds a WSMessage of type "error" with a string body.
// Used when subscription fails after the upgrade has already succeeded —
// the client gets a structured error before the close frame.
func errorWSMessage(msg string) *model.WSMessage {
	payload, _ := json.Marshal(map[string]string{"message": msg})
	return &model.WSMessage{
		Type:      "error",
		Timestamp: time.Now(),
		Payload:   payload,
	}
}

// eventsSource resolves the source backing /ws/topology. Prefers an
// explicit events mapping, falls back to topology then clusters (most
// configs alias them all to the same source in Phase 1), then to the
// lone registered source. Mirrors topologySource() in cluster.go.
func (h *Handler) eventsSource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(resourceEvents); src != nil {
		return src
	}
	if src := h.Registry.SourceFor(resourceTopology); src != nil {
		return src
	}
	if src := h.Registry.SourceFor(resourceClusters); src != nil {
		return src
	}
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}

// activeWSConnections returns the current count of live /ws/topology
// connections. Exported (lowercase but package-visible) so tests can
// assert clean shutdown — see ws_test.go.
//
//nolint:unused // wired by ws_test.go
func activeWSConnections() int32 {
	return wsActiveConnections.Load()
}

