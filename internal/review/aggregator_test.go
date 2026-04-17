package review_test

import (
	"context"
	"testing"

	"github.com/adortb/adortb-creative-review/internal/provider"
	"github.com/adortb/adortb-creative-review/internal/review"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

func newTestAggregator(p provider.LLMProvider) *review.Aggregator {
	policy := rules.DefaultPolicy()
	prompts := rules.NewPromptLibrary(policy)
	return review.NewAggregator(p, policy, prompts)
}

func TestAggregator_TextOnly_Pass(t *testing.T) {
	agg := newTestAggregator(provider.NewMockProvider())
	res, err := agg.Review(context.Background(), review.CreativeRequest{
		CreativeID:  1,
		Headline:    "Best running shoes",
		Description: "High quality athletic gear",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FinalDecision != provider.DecisionPass {
		t.Errorf("expected pass, got %s", res.FinalDecision)
	}
	if res.Text == nil {
		t.Error("expected text result to be set")
	}
	if res.Image != nil {
		t.Error("expected image result to be nil (no image_url)")
	}
}

func TestAggregator_BannedKeyword_Reject(t *testing.T) {
	agg := newTestAggregator(provider.NewMockProvider())
	res, err := agg.Review(context.Background(), review.CreativeRequest{
		CreativeID:  2,
		Headline:    "Cheap casino games",
		Description: "Win big!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FinalDecision != provider.DecisionReject {
		t.Errorf("expected reject for banned keyword, got %s", res.FinalDecision)
	}
}

func TestAggregator_WithImage(t *testing.T) {
	agg := newTestAggregator(provider.NewMockProvider())
	res, err := agg.Review(context.Background(), review.CreativeRequest{
		CreativeID:  3,
		Headline:    "Great shoes",
		Description: "Buy now",
		ImageURL:    "http://example.com/image.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Image == nil {
		t.Error("expected image result to be set when image_url provided")
	}
}

func TestAggregator_WithVideo(t *testing.T) {
	agg := newTestAggregator(provider.NewMockProvider())
	res, err := agg.Review(context.Background(), review.CreativeRequest{
		CreativeID:     4,
		Headline:       "Amazing video ad",
		Description:    "Watch now",
		VideoURL:       "http://example.com/video.mp4",
		VideoFrameURLs: []string{"http://f1.jpg", "http://f2.jpg", "http://f3.jpg"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Video == nil {
		t.Error("expected video result to be set when video_url provided")
	}
	if res.TotalTokens <= 0 {
		t.Error("expected positive total_tokens")
	}
}

func TestAggregator_WorstDecisionWins(t *testing.T) {
	// Mock: text returns pass, image returns reject
	mockP := provider.NewMockProvider()
	mockP.ImageResult = &provider.ReviewResult{
		Decision:  provider.DecisionReject,
		RiskScore: 0.95,
		Reasons:   []string{"explicit content in image"},
	}

	agg := newTestAggregator(mockP)
	res, err := agg.Review(context.Background(), review.CreativeRequest{
		CreativeID:  5,
		Headline:    "Safe headline",
		Description: "Safe description",
		ImageURL:    "http://example.com/bad.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FinalDecision != provider.DecisionReject {
		t.Errorf("expected reject (from image), got %s", res.FinalDecision)
	}
}

func TestAggregator_ProviderError_FallbackToHuman(t *testing.T) {
	mockP := provider.NewMockProvider()
	mockP.Err = context.DeadlineExceeded

	agg := newTestAggregator(mockP)
	res, err := agg.Review(context.Background(), review.CreativeRequest{
		CreativeID:  6,
		Headline:    "Test",
		Description: "Test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// LLM 错误应降级为人工审核
	if res.FinalDecision != provider.DecisionNeedsHuman {
		t.Errorf("expected needs_human on provider error, got %s", res.FinalDecision)
	}
}
