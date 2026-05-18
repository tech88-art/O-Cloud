// Package api — workload logs handlers (P1-T-301).
//
// Endpoints:
//   - GET /api/v1/workloads/:namespace/:name/logs    → LogPage (REST tail)
//   - GET /ws/logs/:namespace/:name                  → WS stream of LogLine
//
// Contract: see docs/api-contract.yaml /api/v1/workloads/{namespace}/{name}/
// logs + components.schemas.LogPage + the "WebSocket 通道" comment block for
// /ws/logs/{ns}/{name}.
//
// Both endpoints resolve the workload first (404 if missing), so a typo in
// the URL fails fast instead of returning a stream of synthetic lines for a
// non-existent workload.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// logsResourceKey is the Registry.SourceFor mapping key for /logs endpoints.
// Falls back to workloadResourceKey then single-source — same pattern as
// other handlers (see workloadSource / eventsSource).
const logsResourceKey = "logs"

// logsRESTTailMax bounds the `tail` query param at the handler level. Mirrors
// the mock's logTailMax so a tail beyond this returns 400 (rather than the
// mock silently clamping — explicit failure is more debuggable for clients).
const logsRESTTailMax = 2000

// GetWorkloadLogs handles GET /api/v1/workloads/:namespace/:name/logs.
//
// Query params (all optional):
//   - container : filter to one container (case-sensitive exact match)
//   - tail      : number of lines to return (default 200, cap 2000)
//   - since     : ISO 8601 timestamp; passed through to the source. The mock
//                 ignores it (synthesized timeline is anchored at call time);
//                 PHASE-2 real source will honor it.
//
// Returns 200 LogPage on success, 404 if the workload doesn't exist, 400 on
// a malformed tail, 500 otherwise.
func (h *Handler) GetWorkloadLogs(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	if namespace == "" || name == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"namespace and name path parameters are required", nil)
		return
	}

	opts, errResp := parseLogOptions(c)
	if errResp != nil {
		respondError(c, errResp.status, errResp.code, errResp.message, errResp.details)
		return
	}

	src := h.logsSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for logs", nil)
		return
	}

	page, err := src.GetWorkloadLogs(c.Request.Context(), namespace, name, opts)
	if err != nil {
		if errors.Is(err, mocksrc.ErrWorkloadNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound,
				"workload not found", map[string]interface{}{
					"resource":  "workload",
					"namespace": namespace,
					"name":      name,
				})
			return
		}
		h.Logger.Error("GetWorkloadLogs failed",
			zap.String("source", src.Name()),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to get workload logs", map[string]interface{}{
				"source": src.Name(),
			})
		return
	}

	c.JSON(http.StatusOK, page)
}

// parseErr is the internal-only shape we build before calling respondError —
// lets parseLogOptions stay pure (no Gin coupling) so it's unit-testable.
type parseErr struct {
	status  int
	code    string
	message string
	details map[string]interface{}
}

// parseLogOptions extracts LogOptions from query params and validates them.
// Returns (opts, nil) on success or (zero, *parseErr) on failure.
func parseLogOptions(c *gin.Context) (model.LogOptions, *parseErr) {
	opts := model.LogOptions{
		Container: c.Query("container"),
		Since:     c.Query("since"),
	}
	if raw := c.Query("tail"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return model.LogOptions{}, &parseErr{
				status:  http.StatusBadRequest,
				code:    CodeBadRequest,
				message: "tail must be a non-negative integer",
				details: map[string]interface{}{"tail": raw},
			}
		}
		if n > logsRESTTailMax {
			return model.LogOptions{}, &parseErr{
				status:  http.StatusBadRequest,
				code:    CodeBadRequest,
				message: "tail exceeds the maximum allowed",
				details: map[string]interface{}{
					"tail": n,
					"max":  logsRESTTailMax,
				},
			}
		}
		opts.Tail = n
	}
	return opts, nil
}

// WSLogs handles GET /ws/logs/:namespace/:name — upgrades to WebSocket and
// streams synthesized log lines as WSMessage envelopes.
//
// Each frame: WSMessage{type:"log", timestamp:<now>, payload:LogLine}.
//
// Lifecycle mirrors WSTopology (capacity gate → upgrade → reader+writer
// loop → wsActiveConnections decrement on exit). The connection cap is
// shared with /ws/topology so a single client can't exhaust all WS slots
// by piling onto either endpoint.
func (h *Handler) WSLogs(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	if namespace == "" || name == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"namespace and name path parameters are required", nil)
		return
	}

	// Capacity gate before upgrade (same as WSTopology).
	if cur := wsActiveConnections.Load(); cur >= wsMaxConnections {
		h.Logger.Warn("ws logs: connection cap reached",
			zap.Int32("active", cur),
			zap.Int32("max", wsMaxConnections))
		respondError(c, http.StatusServiceUnavailable, CodeInternalError,
			"websocket connection cap reached", nil)
		return
	}

	src := h.logsSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for logs", nil)
		return
	}

	// Resolve the workload BEFORE upgrade so a 404 stays an HTTP 404 (a 404
	// after upgrade would be a confusing immediate close frame with no body).
	if _, err := src.GetWorkloadDetail(c.Request.Context(), namespace, name); err != nil {
		if errors.Is(err, mocksrc.ErrWorkloadNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound,
				"workload not found", map[string]interface{}{
					"resource":  "workload",
					"namespace": namespace,
					"name":      name,
				})
			return
		}
		h.Logger.Error("ws logs: workload lookup failed",
			zap.String("source", src.Name()),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to resolve workload", map[string]interface{}{
				"source": src.Name(),
			})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.Logger.Warn("ws logs: upgrade failed", zap.Error(err))
		return
	}

	wsActiveConnections.Add(1)
	opts := model.LogStreamOptions{
		Container: c.Query("container"),
		Since:     c.Query("since"),
	}
	h.serveWSLogs(c.Request.Context(), conn, src, namespace, name, opts)
}

// serveWSLogs owns one upgraded /ws/logs connection. Mirrors serveWSTopology.
// Wraps each LogLine in a WSMessage{type:"log", ...} envelope so the wire
// shape matches the broader WS contract (same envelope shape as topology
// events) — the frontend can dispatch by `type` without separate handlers.
func (h *Handler) serveWSLogs(
	reqCtx context.Context,
	conn *websocket.Conn,
	src datasource.Source,
	namespace, name string,
	opts model.LogStreamOptions,
) {
	defer func() {
		_ = conn.Close()
		wsActiveConnections.Add(-1)
	}()

	streamCtx, cancel := context.WithCancel(reqCtx)
	defer cancel()

	conn.SetReadLimit(wsMaxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})

	lines, err := src.StreamWorkloadLogs(streamCtx, namespace, name, opts)
	if err != nil {
		h.Logger.Error("ws logs: stream subscribe failed",
			zap.String("source", src.Name()),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err))
		_ = writeJSON(conn, errorWSMessage("failed to subscribe to log stream"))
		return
	}

	go h.readerLoop(streamCtx, conn, cancel)
	h.writerLoopLogs(streamCtx, conn, lines)
}

// writerLoopLogs is the log-specific writer. Differs from writerLoop in that
// it wraps each *LogLine in a WSMessage envelope before serialization (the
// topology writer receives pre-wrapped messages from the source).
func (h *Handler) writerLoopLogs(ctx context.Context, conn *websocket.Conn, lines <-chan *model.LogLine) {
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				h.Logger.Debug("ws logs: ping write failed", zap.Error(err))
				return
			}
		case line, ok := <-lines:
			if !ok {
				_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			// WSMessage.Payload is json.RawMessage so clients can lazy-decode;
			// marshal the LogLine here and stuff the bytes in. A marshal error
			// would mean a broken type, so log+drop the frame and continue
			// rather than tearing down the whole stream.
			payload, err := json.Marshal(line)
			if err != nil {
				h.Logger.Warn("ws logs: log line marshal failed", zap.Error(err))
				continue
			}
			msg := &model.WSMessage{
				Type:      "log",
				Timestamp: time.Now().UTC(),
				Payload:   payload,
			}
			if err := writeJSON(conn, msg); err != nil {
				h.Logger.Debug("ws logs: message write failed", zap.Error(err))
				return
			}
		}
	}
}

// logsSource resolves the source backing /logs endpoints. Prefers an explicit
// logs mapping, falls back to workloads then single-source. Mirrors
// workloadSource / eventsSource.
func (h *Handler) logsSource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(logsResourceKey); src != nil {
		return src
	}
	if src := h.Registry.SourceFor(workloadResourceKey); src != nil {
		return src
	}
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}
