// Package provider 定义 LLM 审核 Provider 接口及通用数据结构。
package provider

import "context"

// Decision 审核决策枚举。
type Decision string

const (
	DecisionPass       Decision = "pass"
	DecisionWarn       Decision = "warn"
	DecisionReject     Decision = "reject"
	DecisionNeedsHuman Decision = "needs_human"
)

// Category 违规类别枚举。
type Category string

const (
	CategoryAdultContent        Category = "adult_content"
	CategoryViolence            Category = "violence"
	CategoryMisleadingClaim     Category = "misleading_claim"
	CategoryHateSpeech          Category = "hate_speech"
	CategoryDrugs               Category = "drugs"
	CategoryWeapons             Category = "weapons"
	CategoryTrademarkInfringement Category = "trademark_infringement"
	CategoryDeceptivePractice   Category = "deceptive_practice"
	CategoryPhishing            Category = "phishing"
	CategoryOther               Category = "other"
)

// ReviewResult LLM 审核结果。
type ReviewResult struct {
	Decision   Decision   `json:"decision"`
	RiskScore  float32    `json:"risk_score"`
	Categories []Category `json:"categories"`
	Reasons    []string   `json:"reasons"`
	Confidence float32    `json:"confidence"`
	TokensUsed int        `json:"tokens_used"`
	// RawResponse 原始 LLM 响应（用于存储和调试）
	RawResponse map[string]any `json:"raw_response,omitempty"`
}

// TextReviewRequest 文案审核请求。
type TextReviewRequest struct {
	Headline    string `json:"headline"`
	Description string `json:"description"`
	LandingURL  string `json:"landing_url,omitempty"`
	Category    string `json:"category,omitempty"`
}

// ImageReviewRequest 图片审核请求。
type ImageReviewRequest struct {
	Context string `json:"context,omitempty"` // 广告上下文
}

// VideoReviewRequest 视频审核请求（通过抽帧后走图片审核）。
type VideoReviewRequest struct {
	FrameURLs []string `json:"frame_urls"`
	Context   string   `json:"context,omitempty"`
}

// LLMProvider LLM 审核 Provider 接口。
// 集成方实现此接口并注入，服务本身不直接持有 API 密钥。
type LLMProvider interface {
	// Name 返回 provider 名称，如 "openai"、"claude"、"mock"。
	Name() string

	// AnalyzeText 对广告文案进行合规审核。
	AnalyzeText(ctx context.Context, req TextReviewRequest) (*ReviewResult, error)

	// AnalyzeImage 对广告图片进行内容审核。
	// imageURL 为可公开访问的图片 URL。
	AnalyzeImage(ctx context.Context, imageURL string, req ImageReviewRequest) (*ReviewResult, error)

	// AnalyzeVideo 对视频抽帧后进行审核。
	AnalyzeVideo(ctx context.Context, videoURL string, req VideoReviewRequest) (*ReviewResult, error)
}
