package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adortb/adortb-creative-review/internal/api"
	"github.com/adortb/adortb-creative-review/internal/provider"
	"github.com/adortb/adortb-creative-review/internal/queue"
	"github.com/adortb/adortb-creative-review/internal/review"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

func newTestHandler() (*api.Handler, *http.ServeMux) {
	policy := rules.DefaultPolicy()
	prompts := rules.NewPromptLibrary(policy)
	mockP := provider.NewMockProvider()
	agg := review.NewAggregator(mockP, policy, prompts)
	hq := queue.NewMemQueue()
	h := api.New(agg, hq, policy)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestHandleReview_Pass(t *testing.T) {
	_, mux := newTestHandler()
	body := map[string]any{
		"creative_id": 1,
		"headline":    "Best running shoes",
		"description": "High quality gear",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/review", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp["decision"] != "pass" {
		t.Errorf("expected pass, got %v", resp["decision"])
	}
}

func TestHandleReview_Reject_BannedKeyword(t *testing.T) {
	_, mux := newTestHandler()
	body := map[string]any{
		"creative_id": 2,
		"headline":    "Win at casino tonight",
		"description": "Best gambling site",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/review", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["decision"] != "reject" {
		t.Errorf("expected reject, got %v", resp["decision"])
	}
}

func TestHandleReview_MissingHeadline(t *testing.T) {
	_, mux := newTestHandler()
	body := map[string]any{"creative_id": 3}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/review", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleGetPolicy(t *testing.T) {
	_, mux := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var cfg map[string]any
	if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if cfg["banned_keywords"] == nil {
		t.Error("expected banned_keywords in policy response")
	}
}

func TestHandleUpdatePolicy(t *testing.T) {
	_, mux := newTestHandler()
	body := map[string]any{
		"banned_keywords": []string{"newbanned"},
		"warn_threshold":  0.35,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/v1/policy", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleHumanQueue_ListAndResolve(t *testing.T) {
	_, mux := newTestHandler()
	ctx := context.Background()
	_ = ctx

	// 先触发一个 needs_human 审核（Mock 返回 needs_human）
	mockBody := map[string]any{
		"creative_id": 10,
		"headline":    "test headline",
		"description": "test desc",
	}
	b, _ := json.Marshal(mockBody)

	// 先查询空队列
	req := httptest.NewRequest(http.MethodGet, "/v1/human-queue?status=pending", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// 提交异步审核入队
	req2 := httptest.NewRequest(http.MethodPost, "/v1/review/async", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w2.Code, w2.Body.String())
	}

	var asyncResp map[string]any
	_ = json.NewDecoder(w2.Body).Decode(&asyncResp)
	reviewID := int(asyncResp["review_id"].(float64))

	// 解决队列项（后台 goroutine 可能已先 resolve，409 也可接受）
	resolveBody := map[string]any{"reviewer_id": 99, "decision": "pass"}
	rb, _ := json.Marshal(resolveBody)
	req3 := httptest.NewRequest(http.MethodPost, "/v1/human-queue/"+itoa(reviewID)+"/resolve", bytes.NewReader(rb))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK && w3.Code != http.StatusConflict {
		t.Errorf("expected 200 or 409, got %d: %s", w3.Code, w3.Body.String())
	}
}

func TestHandleHealth(t *testing.T) {
	_, mux := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func itoa(i int) string {
	return string(rune('0' + i%10))
}
