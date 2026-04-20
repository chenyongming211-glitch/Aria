package cli

import (
	"strings"
	"testing"

	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

func TestIssueNodeCertificateRejectsUnsavedNode(t *testing.T) {
	controller := &Controller{certService: newTestCertService(t)}
	node := &controllerstorage.Node{
		TenantID: uuid.New(),
	}

	_, err := controller.issueNodeCertificate(node, generateCSRPEM(t, "unsaved-node"), nil)
	if err == nil {
		t.Fatal("expected certificate issuance to fail for unsaved node")
	}
	if !strings.Contains(err.Error(), "node id is required") {
		t.Fatalf("expected node id error, got %v", err)
	}
}
