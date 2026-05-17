package api

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// wsUpgrader is the shared websocket upgrader for /ws/* endpoints.
//
// PHASE-1 (P1-T-005): this is a stub. /ws/* routes are NOT mounted yet — see
// router.go. T105 implements /ws/topology, T301 implements /ws/logs/*. The
// upgrader sits here now so the dependency is real (rather than parked as a
// dangling go.mod entry) and so later tasks have a single configuration point.
//
//nolint:gochecknoglobals,unused // intentional package-level upgrader (gorilla idiom); consumed by T105 ws handlers
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// PHASE-1: permissive. PHASE-2 will narrow once auth lands.
		return true
	},
}
