package im

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubAIService struct{}

func (stubAIService) Chat(context.Context, string, string) (string, error) {
	return "ok", nil
}

func (stubAIService) ChatWithContext(context.Context, string, string) (string, error) {
	return "ok", nil
}

func (stubAIService) ExecuteTool(context.Context, string, string, map[string]any, bool) (map[string]any, error) {
	return map[string]any{}, nil
}

func TestFeishuWebhookRejectsMissingVerifyToken(t *testing.T) {
	handler := NewFeishuHandler(stubAIService{}, "", "", "", "expected-token")
	body := `{"type":"url_verification","challenge":"challenge-1"}`
	req := httptest.NewRequest(http.MethodPost, "/feishu", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	handler.HandleWebhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing verify token, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFeishuWebhookAcceptsTopLevelVerifyTokenForURLVerification(t *testing.T) {
	handler := NewFeishuHandler(stubAIService{}, "", "", "", "expected-token")
	body := `{"type":"url_verification","challenge":"challenge-1","token":"expected-token"}`
	req := httptest.NewRequest(http.MethodPost, "/feishu", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	handler.HandleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid verify token, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("challenge-1")) {
		t.Fatalf("expected challenge response, got %s", rr.Body.String())
	}
}
