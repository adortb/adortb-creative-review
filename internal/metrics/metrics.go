// Package metrics 提供 Prometheus 指标定义。
package metrics

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Counters 服务核心指标（轻量级，生产环境应替换为 prometheus 库）。
type Counters struct {
	ReviewsTotal    atomic.Int64
	ReviewsPass     atomic.Int64
	ReviewsWarn     atomic.Int64
	ReviewsReject   atomic.Int64
	ReviewsHuman    atomic.Int64
	ReviewsError    atomic.Int64
	TokensUsed      atomic.Int64

	costMu   sync.Mutex
	CostUSD  float64
}

var global = &Counters{}

// Global 返回全局指标实例。
func Global() *Counters { return global }

// RecordDecision 记录审核决策。
func (c *Counters) RecordDecision(decision, provider string, tokens int, costUSD float64) {
	c.ReviewsTotal.Add(1)
	c.TokensUsed.Add(int64(tokens))
	c.costMu.Lock()
	c.CostUSD += costUSD
	c.costMu.Unlock()

	switch decision {
	case "pass":
		c.ReviewsPass.Add(1)
	case "warn":
		c.ReviewsWarn.Add(1)
	case "reject":
		c.ReviewsReject.Add(1)
	case "needs_human":
		c.ReviewsHuman.Add(1)
	}
	_ = provider
}

// RecordError 记录审核错误。
func (c *Counters) RecordError() {
	c.ReviewsError.Add(1)
}

// Snapshot 返回当前指标快照（用于 /metrics 端点）。
func (c *Counters) Snapshot() string {
	c.costMu.Lock()
	cost := c.CostUSD
	c.costMu.Unlock()

	return fmt.Sprintf(`# HELP creative_reviews_total Total review requests
# TYPE creative_reviews_total counter
creative_reviews_total %d

# HELP creative_reviews_by_decision Reviews by decision
# TYPE creative_reviews_by_decision counter
creative_reviews_by_decision{decision="pass"} %d
creative_reviews_by_decision{decision="warn"} %d
creative_reviews_by_decision{decision="reject"} %d
creative_reviews_by_decision{decision="needs_human"} %d
creative_reviews_by_decision{decision="error"} %d

# HELP creative_review_tokens_total Total LLM tokens used
# TYPE creative_review_tokens_total counter
creative_review_tokens_total %d

# HELP creative_review_cost_usd_total Total LLM cost in USD
# TYPE creative_review_cost_usd_total counter
creative_review_cost_usd_total %.6f
`,
		c.ReviewsTotal.Load(),
		c.ReviewsPass.Load(),
		c.ReviewsWarn.Load(),
		c.ReviewsReject.Load(),
		c.ReviewsHuman.Load(),
		c.ReviewsError.Load(),
		c.TokensUsed.Load(),
		cost,
	)
}
