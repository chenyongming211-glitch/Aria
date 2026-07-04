package im

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDingTalkWebhookRejectsWhenSecretNotConfigured(t *testing.T) {
	handler := NewDingTalkHandler(stubAIService{}, "", "")
	req := httptest.NewRequest(http.MethodPost, "/dingtalk", bytes.NewBufferString(`{"content":{"text":"hello"}}`))
	rr := httptest.NewRecorder()

	handler.HandleWebhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when DingTalk secret is not configured, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDingTalkWebhookRejectsInvalidSignature(t *testing.T) {
	handler := NewDingTalkHandler(stubAIService{}, "", "expected-secret")
	req := httptest.NewRequest(http.MethodPost, "/dingtalk", bytes.NewBufferString(`{"content":{"text":"hello"}}`))
	req.Header.Set("timestamp", "1700000000000")
	req.Header.Set("sign", "invalid")
	rr := httptest.NewRecorder()

	handler.HandleWebhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid DingTalk signature, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDingTalkWriteJSONSetsResponseShape(t *testing.T) {
	rr := httptest.NewRecorder()

	writeDingTalkJSON(rr, http.StatusAccepted, map[string]string{"status": "accepted"})

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json content type, got %q", rr.Header().Get("Content-Type"))
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("accepted")) {
		t.Fatalf("expected JSON payload, got %s", rr.Body.String())
	}
}
