package provider

import (
	"context"
	"strings"
)

// MockProvider 用于测试的 Mock Provider，可配置固定返回值或关键词检测逻辑。
type MockProvider struct {
	TextResult  *ReviewResult
	ImageResult *ReviewResult
	VideoResult *ReviewResult
	Err         error

	// bannedWords 关键词触发 reject（可覆盖 TextResult）
	bannedWords []string
}

// NewMockProvider 创建 MockProvider，默认返回 pass。
func NewMockProvider() *MockProvider {
	pass := &ReviewResult{Decision: DecisionPass, RiskScore: 0.0, Confidence: 0.95}
	return &MockProvider{
		TextResult:  pass,
		ImageResult: pass,
		VideoResult: pass,
		bannedWords: []string{"meds", "drugs", "casino", "porn", "hack", "scam"},
	}
}

func (m *MockProvider) Name() string { return "mock" }

func (m *MockProvider) AnalyzeText(ctx context.Context, req TextReviewRequest) (*ReviewResult, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	text := strings.ToLower(req.Headline + " " + req.Description)
	for _, w := range m.bannedWords {
		if strings.Contains(text, w) {
			return &ReviewResult{
				Decision:   DecisionReject,
				RiskScore:  0.95,
				Categories: []Category{CategoryMisleadingClaim},
				Reasons:    []string{"banned keyword: " + w},
				Confidence: 0.99,
				TokensUsed: 50,
			}, nil
		}
	}
	r := *m.TextResult
	r.TokensUsed = 50
	return &r, nil
}

func (m *MockProvider) AnalyzeImage(_ context.Context, imageURL string, _ ImageReviewRequest) (*ReviewResult, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	r := *m.ImageResult
	r.TokensUsed = 200
	r.RawResponse = map[string]any{"image_url": imageURL}
	return &r, nil
}

func (m *MockProvider) AnalyzeVideo(_ context.Context, videoURL string, req VideoReviewRequest) (*ReviewResult, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	r := *m.VideoResult
	r.TokensUsed = len(req.FrameURLs) * 200
	r.RawResponse = map[string]any{"video_url": videoURL, "frames": len(req.FrameURLs)}
	return &r, nil
}
