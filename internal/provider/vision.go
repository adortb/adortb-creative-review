package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseReviewJSON 解析 LLM 返回的 JSON 格式审核结果。
// LLM 可能在 JSON 外包裹 markdown 代码块，此处做容错处理。
func parseReviewJSON(raw string) (*ReviewResult, error) {
	raw = strings.TrimSpace(raw)
	// 去除 ```json ... ``` 包装
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 {
			raw = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var payload struct {
		Decision   string   `json:"decision"`
		RiskScore  float32  `json:"risk_score"`
		Categories []string `json:"categories"`
		Reasons    []string `json:"reasons"`
		Confidence float32  `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("parse review json: %w (raw: %.200s)", err, raw)
	}

	d := Decision(payload.Decision)
	if d != DecisionPass && d != DecisionWarn && d != DecisionReject && d != DecisionNeedsHuman {
		d = DecisionNeedsHuman
	}

	cats := make([]Category, 0, len(payload.Categories))
	for _, c := range payload.Categories {
		cats = append(cats, Category(c))
	}

	return &ReviewResult{
		Decision:    d,
		RiskScore:   clamp(payload.RiskScore),
		Categories:  cats,
		Reasons:     payload.Reasons,
		Confidence:  clamp(payload.Confidence),
		RawResponse: map[string]any{"raw": raw},
	}, nil
}

// mergeWorst 合并两个审核结果，取决策更严格的一个。
// 严格程度：reject > needs_human > warn > pass。
func mergeWorst(a, b *ReviewResult) *ReviewResult {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if severity(b.Decision) > severity(a.Decision) {
		merged := *b
		merged.TokensUsed += a.TokensUsed
		merged.Reasons = append(merged.Reasons, a.Reasons...)
		merged.Categories = mergeCategories(merged.Categories, a.Categories)
		return &merged
	}
	merged := *a
	merged.TokensUsed += b.TokensUsed
	merged.Reasons = append(merged.Reasons, b.Reasons...)
	merged.Categories = mergeCategories(merged.Categories, b.Categories)
	if b.RiskScore > merged.RiskScore {
		merged.RiskScore = b.RiskScore
	}
	return &merged
}

func severity(d Decision) int {
	switch d {
	case DecisionPass:
		return 0
	case DecisionWarn:
		return 1
	case DecisionNeedsHuman:
		return 2
	case DecisionReject:
		return 3
	default:
		return 0
	}
}

func mergeCategories(a, b []Category) []Category {
	seen := make(map[Category]struct{}, len(a)+len(b))
	out := make([]Category, 0, len(a)+len(b))
	for _, c := range append(a, b...) {
		if _, ok := seen[c]; !ok {
			seen[c] = struct{}{}
			out = append(out, c)
		}
	}
	return out
}

func clamp(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
