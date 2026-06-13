package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestRunNetworkListUsesTenantScopedAPIWithBearerToken(t *testing.T) {
	const tenantID = "11111111-1111-1111-1111-111111111111"
	const token = "cli-jwt-token"

	oldControllerURL := controllerURL
	oldAuthToken := authToken
	oldTenantEnv := os.Getenv("ARIACTL_TENANT_ID")
	t.Cleanup(func() {
		controllerURL = oldControllerURL
		authToken = oldAuthToken
		os.Setenv("ARIACTL_TENANT_ID", oldTenantEnv)
	})

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/tenants/"+tenantID+"/nodes" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": []map[string]interface{}{
				{
					"public_key":        "pub-a",
					"hostname":          "node-a",
					"assigned_ip":       "100.64.0.2",
					"advertised_routes": []string{"10.10.0.0/24"},
				},
			},
			"message": "1 nodes retrieved",
			"code":    "SUCCESS",
		})
	}))
	defer server.Close()

	controllerURL = server.URL
	authToken = token
	os.Setenv("ARIACTL_TENANT_ID", tenantID)

	if err := runNetworkList(nil, nil); err != nil {
		t.Fatalf("runNetworkList returned error: %v", err)
	}
	if !called {
		t.Fatal("expected tenant-scoped nodes endpoint to be called")
	}
}
