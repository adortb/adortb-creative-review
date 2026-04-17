package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/adortb/adortb-creative-review/internal/rules"
)

// ClaudeProvider Anthropic Claude 3 Provider 实现。
type ClaudeProvider struct {
	apiKey     string
	model      string // 默认 claude-3-5-sonnet-20241022
	httpClient *http.Client
	prompts    *rules.PromptLibrary
}

// ClaudeOption 配置选项。
type ClaudeOption func(*ClaudeProvider)

// WithClaudeModel 覆盖模型。
func WithClaudeModel(model string) ClaudeOption {
	return func(p *ClaudeProvider) { p.model = model }
}

// NewClaudeProvider 创建 Claude Provider。
func NewClaudeProvider(apiKey string, lib *rules.PromptLibrary, opts ...ClaudeOption) *ClaudeProvider {
	p := &ClaudeProvider{
		apiKey:     apiKey,
		model:      "claude-sonnet-4-6",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		prompts:    lib,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) AnalyzeText(ctx context.Context, req TextReviewRequest) (*ReviewResult, error) {
	prompt := p.prompts.TextReviewPrompt(req.Headline, req.Description, req.LandingURL, req.Category)
	return p.messages(ctx, []claudeMessage{
		{Role: "user", Content: prompt},
	})
}

func (p *ClaudeProvider) AnalyzeImage(ctx context.Context, imageURL string, req ImageReviewRequest) (*ReviewResult, error) {
	prompt := p.prompts.ImageReviewPrompt(req.Context)
	content := []map[string]any{
		{
			"type": "image",
			"source": map[string]string{
				"type":      "url",
				"url":       imageURL,
				"media_type": "image/jpeg",
			},
		},
		{"type": "text", "text": prompt},
	}
	return p.messagesRaw(ctx, []map[string]any{
		{"role": "user", "content": content},
	})
}

func (p *ClaudeProvider) AnalyzeVideo(ctx context.Context, _ string, req VideoReviewRequest) (*ReviewResult, error) {
	var worst *ReviewResult
	for _, frameURL := range req.FrameURLs {
		res, err := p.AnalyzeImage(ctx, frameURL, ImageReviewRequest{Context: req.Context})
		if err != nil {
			return nil, fmt.Errorf("analyze frame %s: %w", frameURL, err)
		}
		worst = mergeWorst(worst, res)
	}
	if worst == nil {
		return &ReviewResult{Decision: DecisionPass, Confidence: 1.0}, nil
	}
	return worst, nil
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *ClaudeProvider) messages(ctx context.Context, msgs []claudeMessage) (*ReviewResult, error) {
	raw := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		raw[i] = map[string]any{"role": m.Role, "content": m.Content}
	}
	return p.messagesRaw(ctx, raw)
}

func (p *ClaudeProvider) messagesRaw(ctx context.Context, msgs []map[string]any) (*ReviewResult, error) {
	sysPrompt := "You are an ad review AI. Always respond with valid JSON matching the required schema."
	body := map[string]any{
		"model":      p.model,
		"max_tokens": 1024,
		"system":     sysPrompt,
		"messages":   msgs,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("claude status %d: %s", resp.StatusCode, string(raw))
	}

	var apiResp claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode claude response: %w", err)
	}
	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("claude returned empty content")
	}

	result, err := parseReviewJSON(apiResp.Content[0].Text)
	if err != nil {
		return nil, err
	}
	result.TokensUsed = apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens
	return result, nil
}
