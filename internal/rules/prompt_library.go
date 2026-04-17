// Package rules 提供 Prompt 模板库和广告政策规则。
package rules

import "fmt"

// PromptLibrary 管理各类审核的 Prompt 模板。
type PromptLibrary struct {
	policy *Policy
}

// NewPromptLibrary 创建默认 Prompt 库。
func NewPromptLibrary(policy *Policy) *PromptLibrary {
	return &PromptLibrary{policy: policy}
}

// TextReviewPrompt 生成文案审核 Prompt。
func (l *PromptLibrary) TextReviewPrompt(headline, description, landingURL, category string) string {
	bannedList := l.policy.BannedKeywords()
	return fmt.Sprintf(`You are an ad review AI. Analyze the following ad copy for policy violations.

Categories to check: misleading_claim, adult_content, hate_speech, violence, drugs, weapons, trademark_infringement, deceptive_practice.

Globally banned keywords (auto-reject if found): %v

Ad Copy:
  Headline: %q
  Description: %q
  Landing URL: %q
  Category: %q

Output ONLY valid JSON (no markdown):
{"decision":"pass|warn|reject|needs_human","risk_score":0.0,"categories":[],"reasons":[],"confidence":0.0}

- decision: pass=compliant, warn=low risk, reject=clear violation, needs_human=uncertain
- risk_score: 0.0 (safe) to 1.0 (high risk)
- confidence: 0.0 to 1.0 (your certainty)`,
		bannedList, headline, description, landingURL, category)
}

// ImageReviewPrompt 生成图片审核 Prompt。
func (l *PromptLibrary) ImageReviewPrompt(context string) string {
	return fmt.Sprintf(`Analyze this ad image for policy violations.
Context: %q

Check for: adult_content, violence, weapons, drugs, hate_speech, unauthorized_celebrity_likeness, misleading_visuals, trademark_infringement.

Output ONLY valid JSON (no markdown):
{"decision":"pass|warn|reject|needs_human","risk_score":0.0,"categories":[],"reasons":[],"confidence":0.0}`, context)
}

// LandingPagePrompt 生成着陆页扫描 Prompt。
func (l *PromptLibrary) LandingPagePrompt(url, pageContent string) string {
	if len(pageContent) > 2000 {
		pageContent = pageContent[:2000] + "...(truncated)"
	}
	return fmt.Sprintf(`You are a web safety AI. Analyze the following landing page for ad policy violations.

URL: %q
Page Content (excerpt):
%s

Check for: phishing, malware, deceptive_practice, misleading_claim, adult_content.

Output ONLY valid JSON (no markdown):
{"decision":"pass|warn|reject|needs_human","risk_score":0.0,"categories":[],"reasons":[],"confidence":0.0}`,
		url, pageContent)
}
