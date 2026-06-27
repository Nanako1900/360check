package problem

import "github.com/nnkglobal/c5-backend/internal/gen/oapi"

// Processing-log action literals. STATUS_CHANGE is backend-only (D3): it is
// appended atomically by the service when a PUT changes status_item_id, and is
// NEVER acceptable on POST /problems/{id}/logs. Clients may only post COMMENT or
// REASSIGN.
const (
	actionStatusChange = string(oapi.ProcessingActionSTATUSCHANGE)
	actionComment      = string(oapi.ProcessingActionCOMMENT)
	actionReassign     = string(oapi.ProcessingActionREASSIGN)
)

// validClientLogAction reports whether action is one a client may POST to the
// processing log. Only COMMENT and REASSIGN are allowed; STATUS_CHANGE (and any
// unknown literal) is rejected so the audit trail's status transitions stay
// backend-authored (D3).
func validClientLogAction(action string) bool {
	switch action {
	case actionComment, actionReassign:
		return true
	default:
		return false
	}
}
