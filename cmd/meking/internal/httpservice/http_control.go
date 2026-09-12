package httpservice

import (
	"errors"
	"net/http"
	"time"

	"github.com/memoria-space/meking/controlplane"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
)

type httpControlActionStatus struct {
	ZoneID       string            `json:"zone_id"`
	Action       string            `json:"action"`
	PendingCount uint64            `json:"pending_count"`
	PendingSince time.Time         `json:"pending_since,omitzero"`
	Policy       httpControlPolicy `json:"policy"`
}

type httpControlActionStatusList struct {
	Actions []httpControlActionStatus `json:"actions"`
}

type httpControlPolicy struct {
	Action         string    `json:"action"`
	Mode           string    `json:"mode"`
	MinimumPending uint64    `json:"minimum_pending"`
	MaximumWait    string    `json:"maximum_wait"`
	Revision       uint64    `json:"revision"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type httpControlPolicyList struct {
	Policies []httpControlPolicy `json:"policies"`
}

type httpControlInvocation struct {
	Action string `json:"action"`
}

type httpPublishControlPolicy struct {
	Mode             string `json:"mode"`
	MinimumPending   uint64 `json:"minimum_pending"`
	MaximumWait      string `json:"maximum_wait"`
	ExpectedRevision uint64 `json:"expected_revision"`
}

func (application *httpApplication) controlActions(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if _, err := requestZoneID(request); err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	statuses, err := application.dependencies.listActionStatuses(request.Context())
	if err != nil {
		writeControlError(writer, err)
		return
	}
	result := httpControlActionStatusList{
		Actions: make([]httpControlActionStatus, len(statuses)),
	}
	for index, status := range statuses {
		result.Actions[index] = projectControlActionStatus(status)
	}
	writeHTTPJSON(writer, http.StatusOK, result)
}

func (application *httpApplication) controlAction(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if _, err := requestZoneID(request); err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	action, err := controlplane.ParseAction(request.PathValue("action"))
	if err != nil {
		writeControlError(writer, err)
		return
	}
	status, err := application.dependencies.readActionStatus(request.Context(), action)
	if err != nil {
		writeControlError(writer, err)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, projectControlActionStatus(status))
}

func (application *httpApplication) invokeControlAction(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if _, err := requestZoneID(request); err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	action, err := controlplane.ParseAction(request.PathValue("action"))
	if err != nil {
		writeControlError(writer, err)
		return
	}
	invocation, err := application.dependencies.invokeAction(request.Context(), action)
	if err != nil {
		writeControlError(writer, err)
		return
	}
	writeHTTPJSON(writer, http.StatusAccepted, httpControlInvocation{
		Action: string(invocation.Action),
	})
}

func (application *httpApplication) controlPolicies(
	writer http.ResponseWriter,
	request *http.Request,
) {
	policies, err := application.dependencies.listPolicies(request.Context())
	if err != nil {
		writeControlError(writer, err)
		return
	}
	result := httpControlPolicyList{
		Policies: make([]httpControlPolicy, len(policies)),
	}
	for index, policy := range policies {
		result.Policies[index] = projectControlPolicy(policy)
	}
	writeHTTPJSON(writer, http.StatusOK, result)
}

func (application *httpApplication) controlPolicy(
	writer http.ResponseWriter,
	request *http.Request,
) {
	action, err := controlplane.ParseAction(request.PathValue("action"))
	if err != nil {
		writeControlError(writer, err)
		return
	}
	policy, err := application.dependencies.readPolicy(request.Context(), action)
	if err != nil {
		writeControlError(writer, err)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, projectControlPolicy(policy))
}

func (application *httpApplication) publishControlPolicy(
	writer http.ResponseWriter,
	request *http.Request,
) {
	action, err := controlplane.ParseAction(request.PathValue("action"))
	if err != nil {
		writeControlError(writer, err)
		return
	}
	var input httpPublishControlPolicy
	if err := decodeHTTPJSON(writer, request, &input); err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code:    "invalid_input",
			Message: err.Error(),
		})
		return
	}
	maximumWait := time.Duration(0)
	if input.MaximumWait != "" {
		maximumWait, err = time.ParseDuration(input.MaximumWait)
		if err != nil {
			writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
				Code:    "invalid_input",
				Message: "maximum_wait must be a valid duration",
			})
			return
		}
	}
	policy, err := application.dependencies.publishPolicy(
		request.Context(),
		controlapplication.PublishPolicyInput{
			Action:           action,
			Mode:             controlplane.PolicyMode(input.Mode),
			MinimumPending:   input.MinimumPending,
			MaximumWait:      maximumWait,
			ExpectedRevision: input.ExpectedRevision,
			PublishedAt:      time.Now().UTC(),
		},
	)
	if err != nil {
		writeControlError(writer, err)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, projectControlPolicy(policy))
}

func projectControlActionStatus(status controlapplication.ActionStatus) httpControlActionStatus {
	return httpControlActionStatus{
		ZoneID:       string(status.ZoneID),
		Action:       string(status.Action),
		PendingCount: status.PendingCount,
		PendingSince: status.PendingSince,
		Policy:       projectControlPolicy(status.Policy),
	}
}

func projectControlPolicy(policy controlplane.Policy) httpControlPolicy {
	return httpControlPolicy{
		Action:         string(policy.Action),
		Mode:           string(policy.Mode),
		MinimumPending: policy.MinimumPending,
		MaximumWait:    policy.MaximumWait.String(),
		Revision:       policy.Revision,
		UpdatedAt:      policy.UpdatedAt,
	}
}

func writeControlError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, controlplane.ErrInvalidAction),
		errors.Is(err, controlplane.ErrInvalidPolicy),
		errors.Is(err, controlplane.ErrInvalidPendingInput):
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code:    "invalid_input",
			Message: "Control Plane input is invalid",
		})
	case errors.Is(err, controlplane.ErrPolicyNotFound):
		writeHTTPErrorValue(writer, http.StatusNotFound, httpError{
			Code:    "control_policy_not_found",
			Message: "Control Policy was not found",
		})
	case errors.Is(err, controlplane.ErrPolicyConflict):
		writeHTTPErrorValue(writer, http.StatusConflict, httpError{
			Code:    "control_policy_conflict",
			Message: "Control Policy revision conflicts with current state",
		})
	case errors.Is(err, controlplane.ErrNoPendingInput):
		writeHTTPErrorValue(writer, http.StatusConflict, httpError{
			Code:    "no_pending_input",
			Message: "Action has no pending input",
		})
	case errors.Is(err, controlplane.ErrActionNotInvocable):
		writeHTTPErrorValue(writer, http.StatusConflict, httpError{
			Code:    "action_not_invocable",
			Message: "Action Policy does not allow manual invocation",
		})
	default:
		writeHTTPErrorResponse(writer, err, false)
	}
}
