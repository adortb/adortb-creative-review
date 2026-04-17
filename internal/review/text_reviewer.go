// Package review 提供各类素材审核逻辑。
package review

import (
	"context"
	"fmt"

	"github.com/adortb/adortb-creative-review/internal/provider"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

// TextReviewer 广告文案审核器。
type TextReviewer struct {
	llm    provider.LLMProvider
	policy *rules.Policy
}

// NewTextReviewer 创建文案审核器。
func NewTextReviewer(llm provider.LLMProvider, policy *rules.Policy) *TextReviewer {
	return &TextReviewer{llm: llm, policy: policy}
}

// Review 审核广告文案。
// 先做无 LLM 的关键词粗筛，再调用 LLM 深度分析（可通过 Redis 缓存跳过）。
func (r *TextReviewer) Review(ctx context.Context, req provider.TextReviewRequest) (*provider.ReviewResult, error) {
	// 粗筛：禁用词命中直接 reject，无需消耗 LLM Token
	if found, kw := r.policy.ContainsBannedKeyword(req.Headline + " " + req.Description); found {
		return &provider.ReviewResult{
			Decision:   provider.DecisionReject,
			RiskScore:  1.0,
			Categories: []provider.Category{provider.CategoryMisleadingClaim},
			Reasons:    []string{fmt.Sprintf("banned keyword found: %q", kw)},
			Confidence: 1.0,
		}, nil
	}

	result, err := r.llm.AnalyzeText(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("text review llm: %w", err)
	}
	return applyThresholds(result, r.policy), nil
}
