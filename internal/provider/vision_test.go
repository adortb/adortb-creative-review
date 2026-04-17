package provider

import (
	"testing"
)

func TestParseReviewJSON_Valid(t *testing.T) {
	raw := `{"decision":"reject","risk_score":0.9,"categories":["adult_content"],"reasons":["explicit content"],"confidence":0.95}`
	res, err := parseReviewJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != DecisionReject {
		t.Errorf("expected reject, got %s", res.Decision)
	}
	if res.RiskScore != 0.9 {
		t.Errorf("expected 0.9, got %f", res.RiskScore)
	}
	if len(res.Categories) != 1 || res.Categories[0] != CategoryAdultContent {
		t.Errorf("unexpected categories: %v", res.Categories)
	}
}

func TestParseReviewJSON_MarkdownWrapped(t *testing.T) {
	raw := "```json\n{\"decision\":\"pass\",\"risk_score\":0.1,\"categories\":[],\"reasons\":[],\"confidence\":0.9}\n```"
	res, err := parseReviewJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != DecisionPass {
		t.Errorf("expected pass, got %s", res.Decision)
	}
}

func TestParseReviewJSON_InvalidDecision(t *testing.T) {
	raw := `{"decision":"unknown","risk_score":0.5,"categories":[],"reasons":[],"confidence":0.8}`
	res, err := parseReviewJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 无效 decision 应降级为 needs_human
	if res.Decision != DecisionNeedsHuman {
		t.Errorf("expected needs_human for unknown decision, got %s", res.Decision)
	}
}

func TestMergeWorst(t *testing.T) {
	pass := &ReviewResult{Decision: DecisionPass, RiskScore: 0.1, TokensUsed: 50}
	reject := &ReviewResult{Decision: DecisionReject, RiskScore: 0.95, TokensUsed: 100, Reasons: []string{"r1"}}

	result := mergeWorst(pass, reject)
	if result.Decision != DecisionReject {
		t.Errorf("expected reject, got %s", result.Decision)
	}
	// tokens 应累加
	if result.TokensUsed != 150 {
		t.Errorf("expected 150 tokens, got %d", result.TokensUsed)
	}
}

func TestMergeWorst_NilHandling(t *testing.T) {
	r := &ReviewResult{Decision: DecisionWarn}
	if mergeWorst(nil, r) != r {
		t.Error("mergeWorst(nil, r) should return r")
	}
	if mergeWorst(r, nil) != r {
		t.Error("mergeWorst(r, nil) should return r")
	}
	if mergeWorst(nil, nil) != nil {
		t.Error("mergeWorst(nil, nil) should return nil")
	}
}

func TestClamp(t *testing.T) {
	cases := []struct{ in, want float32 }{
		{-0.5, 0},
		{0, 0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
	}
	for _, c := range cases {
		if got := clamp(c.in); got != c.want {
			t.Errorf("clamp(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
