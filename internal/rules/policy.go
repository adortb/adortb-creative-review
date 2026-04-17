package rules

import (
	"strings"
	"sync"
)

// Policy 广告政策规则（可动态更新）。
type Policy struct {
	mu              sync.RWMutex
	bannedKeywords  []string
	allowedKeywords []string
	// 各决策阈值（risk_score >= 阈值则升级决策）
	warnThreshold        float32
	rejectThreshold      float32
	humanReviewThreshold float32
}

// PolicyConfig 策略配置。
type PolicyConfig struct {
	BannedKeywords       []string `json:"banned_keywords"`
	AllowedKeywords      []string `json:"allowed_keywords"`
	WarnThreshold        float32  `json:"warn_threshold"`
	RejectThreshold      float32  `json:"reject_threshold"`
	HumanReviewThreshold float32  `json:"human_review_threshold"`
}

// DefaultPolicy 创建默认策略。
func DefaultPolicy() *Policy {
	return &Policy{
		bannedKeywords: []string{
			"casino", "gambling", "porn", "xxx", "hack", "crack",
			"scam", "phishing", "malware", "virus", "trojan",
		},
		allowedKeywords:      []string{},
		warnThreshold:        0.3,
		rejectThreshold:      0.8,
		humanReviewThreshold: 0.5,
	}
}

// BannedKeywords 返回当前禁用词列表（线程安全）。
func (p *Policy) BannedKeywords() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.bannedKeywords))
	copy(out, p.bannedKeywords)
	return out
}

// AllowedKeywords 返回白名单词列表。
func (p *Policy) AllowedKeywords() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.allowedKeywords))
	copy(out, p.allowedKeywords)
	return out
}

// Thresholds 返回当前阈值配置。
func (p *Policy) Thresholds() (warn, reject, human float32) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.warnThreshold, p.rejectThreshold, p.humanReviewThreshold
}

// Update 原子替换策略（线程安全）。
func (p *Policy) Update(cfg PolicyConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cfg.BannedKeywords != nil {
		p.bannedKeywords = cfg.BannedKeywords
	}
	if cfg.AllowedKeywords != nil {
		p.allowedKeywords = cfg.AllowedKeywords
	}
	if cfg.WarnThreshold > 0 {
		p.warnThreshold = cfg.WarnThreshold
	}
	if cfg.RejectThreshold > 0 {
		p.rejectThreshold = cfg.RejectThreshold
	}
	if cfg.HumanReviewThreshold > 0 {
		p.humanReviewThreshold = cfg.HumanReviewThreshold
	}
}

// Config 返回当前策略快照。
func (p *Policy) Config() PolicyConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return PolicyConfig{
		BannedKeywords:       append([]string{}, p.bannedKeywords...),
		AllowedKeywords:      append([]string{}, p.allowedKeywords...),
		WarnThreshold:        p.warnThreshold,
		RejectThreshold:      p.rejectThreshold,
		HumanReviewThreshold: p.humanReviewThreshold,
	}
}

// ContainsBannedKeyword 检查文本是否包含禁用词（粗筛，无需 LLM）。
func (p *Policy) ContainsBannedKeyword(text string) (bool, string) {
	lower := strings.ToLower(text)
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, kw := range p.bannedKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true, kw
		}
	}
	return false, ""
}
