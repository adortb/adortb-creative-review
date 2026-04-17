package review

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adortb/adortb-creative-review/internal/provider"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

// LandingScanner 着陆页安全扫描器。
type LandingScanner struct {
	llm        provider.LLMProvider
	policy     *rules.Policy
	prompts    *rules.PromptLibrary
	httpClient *http.Client
}

// NewLandingScanner 创建着陆页扫描器。
func NewLandingScanner(llm provider.LLMProvider, policy *rules.Policy, prompts *rules.PromptLibrary) *LandingScanner {
	return &LandingScanner{
		llm:    llm,
		policy: policy,
		prompts: prompts,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// Scan 抓取着陆页内容后调用 LLM 进行安全分析。
func (s *LandingScanner) Scan(ctx context.Context, landingURL string) (*provider.ReviewResult, error) {
	if landingURL == "" {
		return nil, fmt.Errorf("landing_url is required")
	}

	pageContent, err := s.fetchPage(ctx, landingURL)
	if err != nil {
		// 抓取失败不直接 reject，推人工审核
		return &provider.ReviewResult{
			Decision:   provider.DecisionNeedsHuman,
			RiskScore:  0.5,
			Categories: []provider.Category{provider.CategoryOther},
			Reasons:    []string{fmt.Sprintf("failed to fetch landing page: %v", err)},
			Confidence: 0.5,
		}, nil
	}

	prompt := s.prompts.LandingPagePrompt(landingURL, pageContent)
	result, err := s.llm.AnalyzeText(ctx, provider.TextReviewRequest{
		Headline:    prompt,
		LandingURL:  landingURL,
	})
	if err != nil {
		return nil, fmt.Errorf("landing scan llm: %w", err)
	}
	return applyThresholds(result, s.policy), nil
}

func (s *LandingScanner) fetchPage(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "AdortbReviewBot/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 最多读取 50KB
	lr := io.LimitReader(resp.Body, 50*1024)
	b, err := io.ReadAll(lr)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	// 简单去除 HTML 标签
	content := strings.TrimSpace(string(b))
	return content, nil
}
