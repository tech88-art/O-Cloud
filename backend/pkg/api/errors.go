package api

import (
	"github.com/gin-gonic/gin"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Canonical error codes mirrored from docs/api-contract.yaml components.responses.
// Keep this list in sync with the contract — handlers must NOT invent new codes
// ad-hoc; add a constant here first.
const (
	CodeBadRequest    = "BadRequest"
	CodeUnauthorized  = "Unauthorized"
	CodeForbidden     = "Forbidden"
	CodeNotFound      = "NotFound"
	CodeConflict      = "Conflict"
	CodeInternalError = "InternalError"
)

// respondError writes a structured Error response in the contract-defined
// shape. err may be nil — the surfaced message is `msg` in that case.
//
// Per backend/CLAUDE.md §4.5: handlers MUST go through this helper rather
// than emitting ad-hoc JSON bodies so the wire format stays uniform.
//
// PHASE-1: T005 mounts only /healthz + /version (both happy-path).
// T101+ resource handlers call this; the helper lives here now so the contract
// for error responses is fixed before any handler is written.
//
//nolint:unused // wired by T101+ handlers
func respondError(c *gin.Context, status int, code string, msg string, details map[string]interface{}) {
	c.AbortWithStatusJSON(status, model.Error{
		Code:    code,
		Message: msg,
		Details: details,
	})
}
