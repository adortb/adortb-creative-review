package review

import (
	"context"
	"fmt"

	"github.com/adortb/adortb-creative-review/internal/provider"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

// ImageReviewer 广告图片审核器。
type ImageReviewer struct {
	llm    provider.LLMProvider
	policy *rules.Policy
}

// NewImageReviewer 创建图片审核器。
func NewImageReviewer(llm provider.LLMProvider, policy *rules.Policy) *ImageReviewer {
	return &ImageReviewer{llm: llm, policy: policy}
}

// Review 审核广告图片。
func (r *ImageReviewer) Review(ctx context.Context, imageURL string, req provider.ImageReviewRequest) (*provider.ReviewResult, error) {
	if imageURL == "" {
		return nil, fmt.Errorf("image_url is required")
	}
	result, err := r.llm.AnalyzeImage(ctx, imageURL, req)
	if err != nil {
		return nil, fmt.Errorf("image review llm: %w", err)
	}
	return applyThresholds(result, r.policy), nil
}
