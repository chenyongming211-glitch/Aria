package v2

import (
	"testing"

	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

func TestBuildTenantNodeResponseDoesNotExposeEnrollmentToken(t *testing.T) {
	router := &Router{}
	node := &controllerstorage.Node{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		PublicKey:         "node-public-key",
		Hostname:          "node-1",
		Status:            "online",
		EnrolledWithToken: "tk_sensitive_enrollment_token",
	}

	response := router.buildTenantNodeResponse(node)

	if _, exists := response["enrolled_with_token"]; exists {
		t.Fatal("tenant node response must not expose enrollment token")
	}
}
