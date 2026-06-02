package victoriametrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryRangeReturnsErrorOnNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad query", http.StatusUnprocessableEntity)
	}))
	t.Cleanup(server.Close)

	_, err := NewClient(server.URL).QueryRange(context.Background(), "bad", time.Now().Add(-time.Hour), time.Now(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("expected status error containing 422, got %v", err)
	}
}

func TestQueryInstantReturnsErrorOnNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad query", http.StatusUnprocessableEntity)
	}))
	t.Cleanup(server.Close)

	_, err := NewClient(server.URL).QueryInstant(context.Background(), "bad")
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("expected status error containing 422, got %v", err)
	}
}
