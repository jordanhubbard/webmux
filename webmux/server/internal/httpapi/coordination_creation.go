package httpapi

import "regexp"

// An uncertain project allocation must not be decoded as a terminal launch.
// The original request remains in the owner's durable coordinator namespace.
type coordinationCreation struct {
	State       string `json:"state"`
	Stage       string `json:"stage"`
	Code        string `json:"code"`
	OperationID string `json:"operationId"`
}

type coordinationCreationPending struct{ Creation coordinationCreation }

func (e *coordinationCreationPending) Error() string { return "Creation is not yet confirmed." }

var creationOperationID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var creationCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,95}$`)

func validCoordinationCreation(value coordinationCreation) bool {
	return value.State == "uncertain" && (value.Stage == "project" || value.Stage == "agent") && creationOperationID.MatchString(value.OperationID) && creationCode.MatchString(value.Code)
}
