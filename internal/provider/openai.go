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

// OpenAIProvider OpenAI GPT-4 / GPT-4V Provider 实现。
// 集成方通过 NewOpenAIProvider 注入 API Key。
type OpenAIProvider struct {
	apiKey     string
	model      string // 默认 gpt-4o
	visionModel string // 默认 gpt-4o（支持视觉）
	httpClient *http.Client
	prompts    *rules.PromptLibrary
}

// OpenAIOption 配置选项。
type OpenAIOption func(*OpenAIProvider)

// WithOpenAIModel 覆盖文本模型。
func WithOpenAIModel(model string) OpenAIOption {
	return func(p *OpenAIProvider) { p.model = model }
}

// WithOpenAIVisionModel 覆盖视觉模型。
func WithOpenAIVisionModel(model string) OpenAIOption {
	return func(p *OpenAIProvider) { p.visionModel = model }
}

// NewOpenAIProvider 创建 OpenAI Provider。
func NewOpenAIProvider(apiKey string, lib *rules.PromptLibrary, opts ...OpenAIOption) *OpenAIProvider {
	p := &OpenAIProvider{
		apiKey:      apiKey,
		model:       "gpt-4o",
		visionModel: "gpt-4o",
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		prompts:     lib,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) AnalyzeText(ctx context.Context, req TextReviewRequest) (*ReviewResult, error) {
	prompt := p.prompts.TextReviewPrompt(req.Headline, req.Description, req.LandingURL, req.Category)
	return p.chatCompletion(ctx, p.model, prompt)
}

func (p *OpenAIProvider) AnalyzeImage(ctx context.Context, imageURL string, req ImageReviewRequest) (*ReviewResult, error) {
	prompt := p.prompts.ImageReviewPrompt(req.Context)
	return p.visionCompletion(ctx, p.visionModel, prompt, imageURL)
}

func (p *OpenAIProvider) AnalyzeVideo(ctx context.Context, _ string, req VideoReviewRequest) (*ReviewResult, error) {
	// 视频通过抽帧后逐帧分析，取最严重结果
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

// chatCompletion 发起文本 Chat Completion。
func (p *OpenAIProvider) chatCompletion(ctx context.Context, model, prompt string) (*ReviewResult, error) {
	body := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": "You are an ad review AI. Always respond with valid JSON."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0,
	}
	return p.doRequest(ctx, body)
}

// visionCompletion 发起多模态 Vision Completion。
func (p *OpenAIProvider) visionCompletion(ctx context.Context, model, textPrompt, imageURL string) (*ReviewResult, error) {
	body := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role": "system",
				"content": "You are an ad image review AI. Always respond with valid JSON.",
			},
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": textPrompt},
					{"type": "image_url", "image_url": map[string]string{"url": imageURL}},
				},
			},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0,
	}
	return p.doRequest(ctx, body)
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *OpenAIProvider) doRequest(ctx context.Context, body map[string]any) (*ReviewResult, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai status %d: %s", resp.StatusCode, string(raw))
	}

	var apiResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	result, err := parseReviewJSON(apiResp.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}
	result.TokensUsed = apiResp.Usage.TotalTokens
	return result, nil
}
