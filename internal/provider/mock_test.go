package provider_test

import (
	"context"
	"testing"

	"github.com/adortb/adortb-creative-review/internal/provider"
)

func TestMockProvider_AnalyzeText_Pass(t *testing.T) {
	p := provider.NewMockProvider()
	res, err := p.AnalyzeText(context.Background(), provider.TextReviewRequest{
		Headline:    "Best running shoes",
		Description: "High quality athletic gear",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != provider.DecisionPass {
		t.Errorf("expected pass, got %s", res.Decision)
	}
}

func TestMockProvider_AnalyzeText_BannedKeyword(t *testing.T) {
	p := provider.NewMockProvider()
	res, err := p.AnalyzeText(context.Background(), provider.TextReviewRequest{
		Headline:    "Buy cheap meds online",
		Description: "Fast delivery",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != provider.DecisionReject {
		t.Errorf("expected reject, got %s", res.Decision)
	}
	if len(res.Reasons) == 0 {
		t.Error("expected reasons for rejection")
	}
}

func TestMockProvider_AnalyzeImage(t *testing.T) {
	p := provider.NewMockProvider()
	res, err := p.AnalyzeImage(context.Background(), "http://example.com/img.jpg", provider.ImageReviewRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected result, got nil")
	}
	if res.TokensUsed != 200 {
		t.Errorf("expected 200 tokens, got %d", res.TokensUsed)
	}
}

func TestMockProvider_AnalyzeVideo(t *testing.T) {
	p := provider.NewMockProvider()
	res, err := p.AnalyzeVideo(context.Background(), "http://example.com/video.mp4", provider.VideoReviewRequest{
		FrameURLs: []string{"http://f1.jpg", "http://f2.jpg", "http://f3.jpg"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 3 帧 × 200 tokens
	if res.TokensUsed != 600 {
		t.Errorf("expected 600 tokens, got %d", res.TokensUsed)
	}
}

func TestMockProvider_ErrorPropagation(t *testing.T) {
	p := provider.NewMockProvider()
	p.Err = context.DeadlineExceeded
	_, err := p.AnalyzeText(context.Background(), provider.TextReviewRequest{Headline: "test"})
	if err == nil {
		t.Error("expected error when Err is set")
	}
}
