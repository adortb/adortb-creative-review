// Package api 提供 HTTP API 处理器。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/adortb/adortb-creative-review/internal/metrics"
	"github.com/adortb/adortb-creative-review/internal/queue"
	"github.com/adortb/adortb-creative-review/internal/review"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

// Handler HTTP 处理器集合。
type Handler struct {
	agg     *review.Aggregator
	hq      queue.Queue
	policy  *rules.Policy
	metrics *metrics.Counters
}

// New 创建 Handler。
func New(agg *review.Aggregator, hq queue.Queue, policy *rules.Policy) *Handler {
	return &Handler{
		agg:     agg,
		hq:      hq,
		policy:  policy,
		metrics: metrics.Global(),
	}
}

// RegisterRoutes 注册所有路由。
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/review", h.handleReview)
	mux.HandleFunc("POST /v1/review/async", h.handleReviewAsync)
	mux.HandleFunc("GET /v1/reviews/{id}", h.handleGetReview)
	mux.HandleFunc("GET /v1/human-queue", h.handleListHumanQueue)
	mux.HandleFunc("POST /v1/human-queue/{id}/resolve", h.handleResolveHumanQueue)
	mux.HandleFunc("GET /v1/policy", h.handleGetPolicy)
	mux.HandleFunc("PUT /v1/policy", h.handleUpdatePolicy)
	mux.HandleFunc("GET /metrics", h.handleMetrics)
	mux.HandleFunc("GET /health", handleHealth)
}

// ReviewRequest POST /v1/review 请求体。
type ReviewRequest struct {
	CreativeID     int64    `json:"creative_id"`
	Headline       string   `json:"headline"`
	Description    string   `json:"description"`
	ImageURL       string   `json:"image_url,omitempty"`
	VideoURL       string   `json:"video_url,omitempty"`
	VideoFrameURLs []string `json:"video_frame_urls,omitempty"`
	LandingURL     string   `json:"landing_url,omitempty"`
	Category       string   `json:"category,omitempty"`
}

// ReviewResponse POST /v1/review 响应。
type ReviewResponse struct {
	CreativeID    int64    `json:"creative_id"`
	Decision      string   `json:"decision"`
	RiskScore     float32  `json:"risk_score"`
	Categories    []string `json:"categories,omitempty"`
	Reasons       []string `json:"reasons,omitempty"`
	TotalTokens   int      `json:"total_tokens"`
	ReviewedAt    string   `json:"reviewed_at"`
}

func (h *Handler) handleReview(w http.ResponseWriter, r *http.Request) {
	var req ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Headline == "" {
		writeError(w, http.StatusBadRequest, "headline is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	agg, err := h.agg.Review(ctx, review.CreativeRequest{
		CreativeID:     req.CreativeID,
		Headline:       req.Headline,
		Description:    req.Description,
		ImageURL:       req.ImageURL,
		VideoURL:       req.VideoURL,
		VideoFrameURLs: req.VideoFrameURLs,
		LandingURL:     req.LandingURL,
		Category:       req.Category,
	})
	if err != nil {
		h.metrics.RecordError()
		writeError(w, http.StatusInternalServerError, "review failed: "+err.Error())
		return
	}

	h.metrics.RecordDecision(string(agg.FinalDecision), "", agg.TotalTokens, 0)

	// 如果需要人工审核，自动入队
	if agg.FinalDecision == "needs_human" {
		_, _ = h.hq.Enqueue(ctx, &queue.HumanReviewItem{
			CreativeID: req.CreativeID,
			Priority:   5,
			Reason:     "AI confidence too low or ambiguous content",
		})
	}

	cats := make([]string, 0)
	var reasons []string
	if agg.Text != nil {
		for _, c := range agg.Text.Categories {
			cats = append(cats, string(c))
		}
		reasons = append(reasons, agg.Text.Reasons...)
	}

	writeJSON(w, http.StatusOK, ReviewResponse{
		CreativeID:  req.CreativeID,
		Decision:    string(agg.FinalDecision),
		RiskScore:   agg.FinalRiskScore,
		Categories:  cats,
		Reasons:     reasons,
		TotalTokens: agg.TotalTokens,
		ReviewedAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

// AsyncReviewResponse POST /v1/review/async 响应（返回 review_id 供后续查询）。
type AsyncReviewResponse struct {
	ReviewID   int64  `json:"review_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

func (h *Handler) handleReviewAsync(w http.ResponseWriter, r *http.Request) {
	var req ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// 入人工队列作为异步占位（生产环境应接 job queue 如 Redis）
	id, err := h.hq.Enqueue(r.Context(), &queue.HumanReviewItem{
		CreativeID: req.CreativeID,
		Priority:   5,
		Reason:     "async review pending",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "enqueue failed")
		return
	}

	// 后台执行审核
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		agg, err := h.agg.Review(ctx, review.CreativeRequest{
			CreativeID:     req.CreativeID,
			Headline:       req.Headline,
			Description:    req.Description,
			ImageURL:       req.ImageURL,
			VideoURL:       req.VideoURL,
			VideoFrameURLs: req.VideoFrameURLs,
			LandingURL:     req.LandingURL,
			Category:       req.Category,
		})
		if err != nil {
			h.metrics.RecordError()
			return
		}
		h.metrics.RecordDecision(string(agg.FinalDecision), "", agg.TotalTokens, 0)
		decision := "pass"
		if agg.FinalDecision != "needs_human" {
			decision = string(agg.FinalDecision)
		}
		_ = h.hq.Resolve(ctx, id, queue.ResolveRequest{
			ReviewerID: 0,
			Decision:   decision,
			Note:       "auto resolved by AI",
		})
	}()

	writeJSON(w, http.StatusAccepted, AsyncReviewResponse{
		ReviewID: id,
		Status:   "pending",
		Message:  "review queued, use GET /v1/reviews/{id} to poll",
	})
}

func (h *Handler) handleGetReview(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.hq.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			writeError(w, http.StatusNotFound, "review not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) handleListHumanQueue(w http.ResponseWriter, r *http.Request) {
	statusStr := r.URL.Query().Get("status")
	if statusStr == "" {
		statusStr = "pending"
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}
	items, err := h.hq.List(r.Context(), queue.Status(statusStr), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) handleResolveHumanQueue(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req queue.ResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Decision == "" {
		writeError(w, http.StatusBadRequest, "decision is required")
		return
	}
	if err := h.hq.Resolve(r.Context(), id, req); err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			writeError(w, http.StatusNotFound, "review item not found")
			return
		}
		if errors.Is(err, queue.ErrAlreadyResolved) {
			writeError(w, http.StatusConflict, "already resolved")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func (h *Handler) handleGetPolicy(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.policy.Config())
}

func (h *Handler) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	var cfg rules.PolicyConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	h.policy.Update(cfg)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprint(w, h.metrics.Snapshot())
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
