package rules_test

import (
	"testing"

	"github.com/adortb/adortb-creative-review/internal/rules"
)

func TestPolicy_ContainsBannedKeyword(t *testing.T) {
	p := rules.DefaultPolicy()

	tests := []struct {
		text  string
		found bool
	}{
		{"buy casino chips now", true},
		{"great running shoes", false},
		{"CASINO games online", true},  // 大小写不敏感
		{"family friendly content", false},
	}
	for _, tt := range tests {
		found, kw := p.ContainsBannedKeyword(tt.text)
		if found != tt.found {
			t.Errorf("ContainsBannedKeyword(%q) = %v (kw=%q), want %v", tt.text, found, kw, tt.found)
		}
	}
}

func TestPolicy_Update(t *testing.T) {
	p := rules.DefaultPolicy()
	p.Update(rules.PolicyConfig{
		BannedKeywords:  []string{"testbanned"},
		WarnThreshold:   0.4,
		RejectThreshold: 0.9,
	})

	found, kw := p.ContainsBannedKeyword("this is testbanned content")
	if !found || kw != "testbanned" {
		t.Errorf("expected testbanned to be found, got found=%v kw=%q", found, kw)
	}

	warnT, rejectT, _ := p.Thresholds()
	if warnT != 0.4 {
		t.Errorf("expected warn threshold 0.4, got %f", warnT)
	}
	if rejectT != 0.9 {
		t.Errorf("expected reject threshold 0.9, got %f", rejectT)
	}
}

func TestPolicy_Config_Immutable(t *testing.T) {
	p := rules.DefaultPolicy()
	cfg := p.Config()
	original := len(cfg.BannedKeywords)

	// 修改 snapshot 不应影响 policy
	cfg.BannedKeywords = append(cfg.BannedKeywords, "injected")
	if len(p.Config().BannedKeywords) != original {
		t.Error("modifying Config() snapshot should not affect policy")
	}
}

func TestPolicy_Thresholds_Default(t *testing.T) {
	p := rules.DefaultPolicy()
	warn, reject, human := p.Thresholds()
	if warn >= reject {
		t.Error("warn threshold should be less than reject threshold")
	}
	if human <= warn || human >= reject {
		t.Error("human threshold should be between warn and reject")
	}
}
