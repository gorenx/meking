package httpservice

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/memoria-space/meking/controlplane"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
	"github.com/memoria-space/meking/zone"
)

func TestHTTPControlActionReturnsDerivedStatus(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	dependencies.readActionStatus = func(
		_ context.Context,
		action controlplane.Action,
	) (controlapplication.ActionStatus, error) {
		return controlapplication.ActionStatus{
			ZoneID:       httpTestZoneID,
			Action:       action,
			PendingCount: 5,
			PendingSince: time.Unix(9, 0).UTC(),
			Policy: controlplane.Policy{
				Action:    action,
				Mode:      controlplane.Manual,
				Revision:  1,
				UpdatedAt: time.Unix(10, 0).UTC(),
			},
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t,
		handler,
		http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/control/actions/extract_knowledge",
		nil,
	)
	var status httpControlActionStatus
	decodeHTTPTestResponse(t, response, &status)
	if response.Code != http.StatusOK || status.PendingCount != 5 ||
		status.Action != string(controlplane.ExtractKnowledge) {
		t.Fatalf("status/ActionStatus = %d/%#v", response.Code, status)
	}
}

func TestHTTPPublishControlPolicyUsesProjectActionScope(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	var published controlapplication.PublishPolicyInput
	dependencies.publishPolicy = func(
		_ context.Context,
		input controlapplication.PublishPolicyInput,
	) (controlplane.Policy, error) {
		published = input
		return controlplane.Policy{
			Action:         input.Action,
			Mode:           input.Mode,
			MinimumPending: input.MinimumPending,
			MaximumWait:    input.MaximumWait,
			Revision:       3,
			UpdatedAt:      input.PublishedAt,
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t,
		handler,
		http.MethodPut,
		"/api/v1/control/policies/derive_community_structure",
		[]byte(`{"mode":"automatic","minimum_pending":4,"maximum_wait":"2m","expected_revision":2}`),
	)
	var policy httpControlPolicy
	decodeHTTPTestResponse(t, response, &policy)
	if response.Code != http.StatusOK || published.Action != controlplane.DeriveCommunityStructure ||
		published.MinimumPending != 4 || published.MaximumWait != 2*time.Minute ||
		published.ExpectedRevision != 2 || policy.Revision != 3 {
		t.Fatalf("status/published/Policy = %d/%#v/%#v", response.Code, published, policy)
	}
}

func TestHTTPInvokeControlActionStartsCurrentZone(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	var invokedZone zone.ID
	dependencies.invokeAction = func(
		ctx context.Context,
		action controlplane.Action,
	) (controlapplication.Invocation, error) {
		var err error
		invokedZone, err = zone.RequireID(ctx)
		if err != nil {
			return controlapplication.Invocation{}, err
		}
		return controlapplication.Invocation{Action: action}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t,
		handler,
		http.MethodPost,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/control/actions/extract_knowledge/invoke",
		nil,
	)
	var invocation httpControlInvocation
	decodeHTTPTestResponse(t, response, &invocation)
	if response.Code != http.StatusAccepted ||
		invocation.Action != string(controlplane.ExtractKnowledge) ||
		invokedZone != httpTestZoneID {
		t.Fatalf("status/Invocation = %d/%#v", response.Code, invocation)
	}
}
