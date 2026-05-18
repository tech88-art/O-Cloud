package model

import (
	"encoding/json"
	"time"
)

// WSMessage is the envelope every /ws/* endpoint emits. Mirrors
// components.schemas.WSMessage in docs/api-contract.yaml (per P1-T-105).
//
// Payload uses json.RawMessage so callers (and clients) decode lazily into
// the per-Type concrete shape — keeps the envelope cheap and avoids forcing
// every event Type through one interface{}.
type WSMessage struct {
	// Type is one of the contract-defined event types, e.g. "topology.update",
	// "workload.statusChanged". Validated against the WSMessage enum in the
	// OpenAPI contract; the mock source emits only types present in
	// configs/mock-data/schema.json Event.type enum.
	Type string `json:"type"`

	// Timestamp is the server clock when the message was emitted (i.e. when
	// the replayer wrote it to the channel — NOT the original event's
	// declared timestamp from events.json). Frontend uses this to drive its
	// own animations; the original-event clock is implicit in the ordering.
	Timestamp time.Time `json:"timestamp"`

	// Payload is the event-specific body, lazy-decoded by clients. The mock
	// replayer copies the bytes verbatim from events.json so what the
	// frontend sees mirrors the fixture (no re-serialization drift).
	Payload json.RawMessage `json:"payload"`
}

// Event is the on-disk shape of one entry in events.json. Lives in model/
// (rather than mock-private) because tests in pkg/api need it to write
// fixture files and assert decoded shapes.
//
// Timestamp is the ISO datetime declared in events.json (e.g.
// "2026-05-17T12:00:00Z"). The replayer computes a delay relative to the
// first event's timestamp; no two events share absolute wall-clock time.
type Event struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// StreamEventsOptions controls the StreamEvents replayer.
//
// Zero value = real-time replay (production behavior). Tests inflate
// FastForward to compress 180s of fixture into <1s.
type StreamEventsOptions struct {
	// FastForward divides every inter-event delay by its value. 0 (zero
	// value) means real-time; tests use a large number (e.g. 1000) to
	// collapse the storyline. Negative values are clamped to 0.
	//
	// Implementation note: we use a divisor (not a fixed dt) so the
	// replayer still honors the relative ordering of events even under
	// fast-forward — the storyline shape stays intact, only its tempo
	// changes.
	FastForward float64
}
