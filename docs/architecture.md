# adortb-creative-review 内部架构

## 内部架构图

```
┌──────────────────────────────────────────────────────────────────┐
│                 adortb-creative-review 内部架构                   │
│                                                                  │
│  HTTP 请求                                                       │
│      │                                                          │
│      ▼                                                          │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  internal/api/handler.go  Handler                        │   │
│  │  POST /v1/review          POST /v1/review/async          │   │
│  │  GET  /v1/reviews/{id}    GET/POST /v1/human-queue       │   │
│  │  GET/PUT /v1/policy       GET /metrics  GET /health      │   │
│  └──────────────────┬─────────────────────────────────────── ┘   │
│                     │                                           │
│                     ▼                                           │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  internal/review/aggregator.go  Aggregator               │   │
│  │                                                          │   │
│  │  Review(ctx, CreativeRequest)                            │   │
│  │  ┌──────────────────────────────────────────────────┐   │   │
│  │  │  并发 goroutine pool                             │   │   │
│  │  │  ├── TextReviewer.Review()    (必执行)           │   │   │
│  │  │  ├── ImageReviewer.Review()   (有 image_url)     │   │   │
│  │  │  ├── VideoReviewer.Review()   (有 video_url)     │   │   │
│  │  │  └── LandingScanner.Scan()    (有 landing_url)   │   │   │
│  │  └──────────────────────────────────────────────────┘   │   │
│  │                                                          │   │
│  │  mergeResultWorst()  取 severity 最高结果                │   │
│  │  applyThresholds()   Policy 阈值覆盖                     │   │
│  └──────────────────────────┬───────────────────────────────┘   │
│                             │                                   │
│                    LLMProvider 调用                              │
│                             │                                   │
│      ┌──────────────────────┼──────────────────────────┐       │
│      │                      │                          │       │
│      ▼                      ▼                          ▼       │
│  ┌──────────┐         ┌──────────┐               ┌──────────┐  │
│  │ provider/ │         │ provider/│               │ provider/│  │
│  │ claude.go │         │ openai  │               │ mock.go  │  │
│  │           │         │ .go     │               │          │  │
│  │ Anthropic │         │ GPT-4o  │               │ 固定返回  │  │
│  │ API       │         │ API     │               │ (测试用) │  │
│  └──────────┘         └──────────┘               └──────────┘  │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  rules/policy.go  Policy (sync.RWMutex)                  │   │
│  │  BannedKeywords + WarnThreshold/RejectThreshold/Human    │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  queue/human_queue.go  HumanQueue（内存实现）             │   │
│  └──────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
```

## 数据流

### 同步审核数据流

```
POST /v1/review {creative_id, headline, description, image_url, video_url, landing_url}
    │
    ▼
Handler.handleReview()
    │
    ├─ Policy.ContainsBannedKeyword(headline+description)
    │   → true: 立即返回 reject（无需 LLM）
    │
    ▼
Aggregator.Review(ctx, CreativeRequest)
    │
    ├── tasks = [text, (image?), (video?), (landing?)]
    │
    ├── 并发 goroutine（每个 task 独立 goroutine）
    │    │
    │    ├── TextReviewer.Review()
    │    │    llm.AnalyzeText(TextReviewRequest{Headline, Description, LandingURL})
    │    │    → PromptLibrary.TextReviewPrompt()
    │    │    → LLM API call → parseReviewJSON()
    │    │    → ReviewResult{Decision, RiskScore, Reasons, TokensUsed}
    │    │
    │    ├── ImageReviewer.Review(imageURL)
    │    │    llm.AnalyzeImage(imageURL, ImageReviewRequest)
    │    │    → Vision API（图片 URL 直接传入）
    │    │    → ReviewResult
    │    │
    │    ├── VideoReviewer.Review(videoURL, frameURLs)
    │    │    for each frame:
    │    │      llm.AnalyzeImage(frameURL) → ReviewResult
    │    │    mergeWorst(results) → 取最严帧的结果
    │    │
    │    └── LandingScanner.Scan(landingURL)
    │         HTTP GET landingURL → 提取文本内容
    │         llm.AnalyzeText() → ReviewResult
    │
    ├── close(results) → range results
    │    applyThresholds(result, policy)  按阈值调整决策
    │    mergeResultWorst(worst, result)  取最严决策
    │
    └── AggregatedResult{FinalDecision, FinalRiskScore, Text, Image, Video, Landing, TotalTokens}
    │
    ├── FinalDecision == needs_human → hq.Push(item)
    └── 返回 ReviewResponse
```

## 时序图

```
广告主     Handler      Aggregator   TextReviewer  ImageReviewer   ClaudeAPI
  │           │              │              │              │            │
  │─POST /v1/review──────────►             │              │            │
  │           │              │──goroutine─►│              │            │
  │           │              │             │──AnalyzeText─────────────►│
  │           │              │──goroutine──────────────── ►│            │
  │           │              │             │──AnalyzeImage────────────►│
  │           │              │             │◄─ReviewResult             │
  │           │              │             │              │◄─ReviewResult
  │           │              │  wait group done           │            │
  │           │              │  mergeResultWorst          │            │
  │           │◄─AggResult───│              │              │            │
  │◄─200 JSON─│              │              │              │            │
```

## 状态机

### ReviewResult 决策转换

```
LLM 返回 ReviewResult{decision, risk_score}
    │
    ▼
applyThresholds(result, policy)
    │
    ├── risk_score >= reject_threshold (0.8)  → DecisionReject
    ├── risk_score >= human_threshold (0.5)
    │   AND decision was Pass               → DecisionNeedsHuman
    ├── risk_score >= warn_threshold (0.3)
    │   AND decision was Pass               → DecisionWarn
    └── else                                → decision 不变
```

### 子任务失败处理

```
goroutine task.fn() 返回 error
    │
    ▼
降级为 ReviewResult{
    Decision:   DecisionNeedsHuman,
    RiskScore:  0.5,
    Reasons:    [err.Error()],
    Confidence: 0.5,
}
不 panic，不中断整体流程
```

### 并发决策聚合

```
mergeResultWorst(a, b *ReviewResult) *ReviewResult
    severity: pass=0, warn=1, needs_human=2, reject=3
    │
    ├── severity(b) > severity(a)    → 返回 b
    ├── b.RiskScore > a.RiskScore    → 返回 a 但用 b 的 RiskScore
    └── else                         → 返回 a
```

### Provider 选择逻辑

```
buildProvider(prompts)
    │
    ├── OPENAI_API_KEY 不为空   → NewOpenAIProvider(key, prompts)
    ├── ANTHROPIC_API_KEY 不为空→ NewClaudeProvider(key, prompts)
    └── 都为空                 → NewMockProvider()（警告日志）
```
