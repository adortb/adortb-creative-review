package review

import (
	"context"
	"fmt"

	"github.com/adortb/adortb-creative-review/internal/provider"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

// VideoReviewer 广告视频审核器（通过抽帧走 ImageReviewer）。
type VideoReviewer struct {
	llm    provider.LLMProvider
	policy *rules.Policy
}

// NewVideoReviewer 创建视频审核器。
func NewVideoReviewer(llm provider.LLMProvider, policy *rules.Policy) *VideoReviewer {
	return &VideoReviewer{llm: llm, policy: policy}
}

// Review 对视频提供的抽帧 URL 进行审核，取最严结果。
// frameURLs 由调用方提供（通常 3 帧），服务本身不做视频解码。
func (r *VideoReviewer) Review(ctx context.Context, videoURL string, frameURLs []string) (*provider.ReviewResult, error) {
	if len(frameURLs) == 0 {
		return nil, fmt.Errorf("at least one frame_url is required for video review")
	}
	result, err := r.llm.AnalyzeVideo(ctx, videoURL, provider.VideoReviewRequest{
		FrameURLs: frameURLs,
	})
	if err != nil {
		return nil, fmt.Errorf("video review llm: %w", err)
	}
	return applyThresholds(result, r.policy), nil
}
