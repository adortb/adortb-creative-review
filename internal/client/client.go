// Package client 提供供 adortb-admin 调用的 HTTP 客户端。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ReviewRequest 审核请求。
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

// ReviewResponse 审核响应。
type ReviewResponse struct {
	CreativeID  int64    `json:"creative_id"`
	Decision    string   `json:"decision"`
	RiskScore   float32  `json:"risk_score"`
	Categories  []string `json:"categories,omitempty"`
	Reasons     []string `json:"reasons,omitempty"`
	TotalTokens int      `json:"total_tokens"`
	ReviewedAt  string   `json:"reviewed_at"`
}

// Client adortb-creative-review 服务客户端。
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New 创建客户端。baseURL 形如 "http://localhost:8104"。
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// Review 同步审核素材。
func (c *Client) Review(ctx context.Context, req ReviewRequest) (*ReviewResponse, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/review", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("creative-review status %d: %s", resp.StatusCode, errResp["error"])
	}

	var result ReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ReviewAsync 异步审核素材，返回 review_id。
func (c *Client) ReviewAsync(ctx context.Context, req ReviewRequest) (int64, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/review/async", bytes.NewReader(b))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		ReviewID int64 `json:"review_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}
	return result.ReviewID, nil
}
