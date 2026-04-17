package review

import (
	"context"
	"sync"

	"github.com/adortb/adortb-creative-review/internal/provider"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

// CreativeRequest 完整素材审核请求。
type CreativeRequest struct {
	CreativeID  int64  `json:"creative_id"`
	Headline    string `json:"headline"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url,omitempty"`
	VideoURL    string `json:"video_url,omitempty"`
	// VideoFrameURLs 视频抽帧 URL（调用方提供，通常 3 帧）
	VideoFrameURLs []string `json:"video_frame_urls,omitempty"`
	LandingURL     string   `json:"landing_url,omitempty"`
	Category       string   `json:"category,omitempty"`
}

// AggregatedResult 聚合审核结果。
type AggregatedResult struct {
	FinalDecision  provider.Decision  `json:"final_decision"`
	FinalRiskScore float32            `json:"final_risk_score"`
	Text           *provider.ReviewResult `json:"text,omitempty"`
	Image          *provider.ReviewResult `json:"image,omitempty"`
	Video          *provider.ReviewResult `json:"video,omitempty"`
	Landing        *provider.ReviewResult `json:"landing,omitempty"`
	TotalTokens    int                `json:"total_tokens"`
}

// Aggregator 协调各子审核器并聚合结果。
type Aggregator struct {
	text    *TextReviewer
	image   *ImageReviewer
	video   *VideoReviewer
	landing *LandingScanner
}

// NewAggregator 创建审核聚合器。
func NewAggregator(llm provider.LLMProvider, policy *rules.Policy, prompts *rules.PromptLibrary) *Aggregator {
	return &Aggregator{
		text:    NewTextReviewer(llm, policy),
		image:   NewImageReviewer(llm, policy),
		video:   NewVideoReviewer(llm, policy),
		landing: NewLandingScanner(llm, policy, prompts),
	}
}

// Review 并行执行所有子审核，聚合最终决策。
func (a *Aggregator) Review(ctx context.Context, req CreativeRequest) (*AggregatedResult, error) {
	type subResult struct {
		kind   string
		result *provider.ReviewResult
		err    error
	}

	tasks := []struct {
		kind string
		fn   func() (*provider.ReviewResult, error)
	}{
		{
			kind: "text",
			fn: func() (*provider.ReviewResult, error) {
				return a.text.Review(ctx, provider.TextReviewRequest{
					Headline:    req.Headline,
					Description: req.Description,
					LandingURL:  req.LandingURL,
					Category:    req.Category,
				})
			},
		},
	}

	if req.ImageURL != "" {
		tasks = append(tasks, struct {
			kind string
			fn   func() (*provider.ReviewResult, error)
		}{
			kind: "image",
			fn: func() (*provider.ReviewResult, error) {
				return a.image.Review(ctx, req.ImageURL, provider.ImageReviewRequest{Context: req.Category})
			},
		})
	}

	if req.VideoURL != "" && len(req.VideoFrameURLs) > 0 {
		tasks = append(tasks, struct {
			kind string
			fn   func() (*provider.ReviewResult, error)
		}{
			kind: "video",
			fn: func() (*provider.ReviewResult, error) {
				return a.video.Review(ctx, req.VideoURL, req.VideoFrameURLs)
			},
		})
	}

	if req.LandingURL != "" {
		tasks = append(tasks, struct {
			kind string
			fn   func() (*provider.ReviewResult, error)
		}{
			kind: "landing",
			fn: func() (*provider.ReviewResult, error) {
				return a.landing.Scan(ctx, req.LandingURL)
			},
		})
	}

	results := make(chan subResult, len(tasks))
	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(kind string, fn func() (*provider.ReviewResult, error)) {
			defer wg.Done()
			r, err := fn()
			results <- subResult{kind: kind, result: r, err: err}
		}(t.kind, t.fn)
	}
	wg.Wait()
	close(results)

	agg := &AggregatedResult{}
	var worst *provider.ReviewResult

	for sr := range results {
		if sr.err != nil {
			// 子任务失败推人工审核
			sr.result = &provider.ReviewResult{
				Decision:   provider.DecisionNeedsHuman,
				RiskScore:  0.5,
				Reasons:    []string{sr.err.Error()},
				Confidence: 0.5,
			}
		}
		switch sr.kind {
		case "text":
			agg.Text = sr.result
		case "image":
			agg.Image = sr.result
		case "video":
			agg.Video = sr.result
		case "landing":
			agg.Landing = sr.result
		}
		agg.TotalTokens += sr.result.TokensUsed
		worst = mergeResultWorst(worst, sr.result)
	}

	if worst == nil {
		worst = &provider.ReviewResult{Decision: provider.DecisionPass, RiskScore: 0}
	}
	agg.FinalDecision = worst.Decision
	agg.FinalRiskScore = worst.RiskScore
	return agg, nil
}

func mergeResultWorst(a, b *provider.ReviewResult) *provider.ReviewResult {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if severity(b.Decision) > severity(a.Decision) {
		return b
	}
	if b.RiskScore > a.RiskScore {
		merged := *a
		merged.RiskScore = b.RiskScore
		return &merged
	}
	return a
}

// applyThresholds 根据 policy 阈值调整 LLM 返回的决策。
func applyThresholds(r *provider.ReviewResult, p *rules.Policy) *provider.ReviewResult {
	if r == nil {
		return r
	}
	warnT, rejectT, humanT := p.Thresholds()
	out := *r
	switch {
	case r.RiskScore >= rejectT:
		out.Decision = provider.DecisionReject
	case r.RiskScore >= humanT && r.Decision == provider.DecisionPass:
		out.Decision = provider.DecisionNeedsHuman
	case r.RiskScore >= warnT && r.Decision == provider.DecisionPass:
		out.Decision = provider.DecisionWarn
	}
	return &out
}

func severity(d provider.Decision) int {
	switch d {
	case provider.DecisionPass:
		return 0
	case provider.DecisionWarn:
		return 1
	case provider.DecisionNeedsHuman:
		return 2
	case provider.DecisionReject:
		return 3
	default:
		return 0
	}
}
