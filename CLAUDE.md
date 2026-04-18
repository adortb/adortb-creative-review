# adortb-creative-review

> adortb 平台 LLM 素材审核服务，并行调用 Claude/OpenAI 对广告文案、图片、视频帧和着陆页进行自动审核，输出风险评分和四级决策（pass/warn/needs_human/reject）。

## 快速理解

- **本项目做什么**：接收广告素材（文案+图片+视频帧+着陆页），并行调用多个 LLM 审核器，聚合最严重的结果作为最终决策，needs_human 时推入人工队列
- **架构位置**：广告素材上线前的内容合规关卡，独立于竞拍链路
- **核心入口**：
  - 服务启动：`cmd/creative-review/main.go`（端口 8104）
  - 并行审核：`internal/review/aggregator.go:Aggregator.Review`
  - LLM 接口：`internal/provider/interface.go:LLMProvider`

## 目录结构

```
adortb-creative-review/
├── cmd/creative-review/main.go # 主程序：Provider 选择（OPENAI>ANTHROPIC>Mock），服务启动
└── internal/
    ├── api/handler.go          # HTTP 路由（/v1/review, /v1/review/async, /v1/human-queue, /v1/policy）
    ├── review/
    │   ├── aggregator.go       # Aggregator.Review：并发调度 + mergeResultWorst
    │   ├── text_reviewer.go    # 文案审核（headline/description/category）
    │   ├── image_reviewer.go   # 图片审核（Vision API）
    │   ├── video_reviewer.go   # 视频审核（逐帧，取最坏结果）
    │   └── landing_scanner.go  # 着陆页扫描（URL 内容 + 关键词）
    ├── provider/
    │   ├── interface.go        # LLMProvider 接口（AnalyzeText/AnalyzeImage/AnalyzeVideo）
    │   ├── claude.go           # Claude 3.5 Sonnet Provider（claude-sonnet-4-6）
    │   ├── openai.go           # GPT-4o Provider
    │   ├── vision.go           # Vision 工具函数（parseReviewJSON）
    │   └── mock.go             # Mock Provider（deterministic，无需 API Key）
    ├── rules/
    │   ├── policy.go           # Policy（禁用词 + 阈值，RWMutex 线程安全动态更新）
    │   └── prompt_library.go   # PromptLibrary（各场景 Prompt 模板）
    └── queue/
        └── human_queue.go      # HumanQueue（内存实现，可替换为 DB/MQ）
```

## 核心概念

### 并行审核（`review/aggregator.go:Aggregator.Review`）

```go
// 并发启动最多 4 个 goroutine（text 必有，其余按请求内容决定）
for _, task := range tasks {
    go func() { results <- task.fn() }()
}
// 子任务失败 → 降级为 needs_human（不 panic，不中断）
// mergeResultWorst：取 severity 最高的决策
severity: pass=0, warn=1, needs_human=2, reject=3
```

### 决策阈值（`rules/policy.go`）

```
risk_score >= reject_threshold (0.8)  → DecisionReject
risk_score >= human_threshold (0.5)   → DecisionNeedsHuman
risk_score >= warn_threshold (0.3)    → DecisionWarn
else                                  → DecisionPass（LLM 原始决策）
```

阈值通过 `PUT /v1/policy` 动态更新（线程安全）。

### LLM Provider 选择（`cmd/main.go:buildProvider`）

优先级：`OPENAI_API_KEY` > `ANTHROPIC_API_KEY` > Mock
- Mock Provider 返回 deterministic 结果，适用于本地开发和 CI
- Claude 使用 `claude-sonnet-4-6`（可通过 `WithClaudeModel` 覆盖）

### Prompt 设计（`rules/prompt_library.go`）

```
system: "You are an ad review AI. Always respond with valid JSON matching the required schema."
user:   [PromptLibrary.TextReviewPrompt / ImageReviewPrompt]
```

LLM 必须返回符合 `ReviewResult` schema 的 JSON（decision/risk_score/reasons/confidence）。

## 开发指南

### Go 版本

```bash
export PATH="$HOME/.goenv/versions/1.25.3/bin:$PATH"
```

### 本地运行（Mock Provider）

```bash
export PORT=8104
go run cmd/creative-review/main.go
# 无需 API Key，自动使用 Mock Provider

curl -X POST http://localhost:8104/v1/review \
  -d '{"creative_id":1,"headline":"限时特惠","description":"正品行货","category":"electronics"}'
```

### 使用 Claude Provider

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
go run cmd/creative-review/main.go
```

### 测试

```bash
go test ./... -cover -race
go test ./internal/review/... -v     # aggregator 测试
go test ./internal/provider/... -v  # provider mock 测试
go test ./internal/rules/... -v     # policy 测试
```

### 添加新的审核维度

1. 在 `review/` 新建 `xxx_reviewer.go`，实现 `func Review(ctx, ...) (*provider.ReviewResult, error)`
2. 在 `aggregator.go:Aggregator` 添加新字段，在 `Review` 中添加新 task
3. 在 `rules/prompt_library.go` 添加对应 Prompt 模板

## 依赖关系

- **上游**：广告主/管理后台（提交素材审核）
- **下游**：Anthropic API / OpenAI API（LLM 调用），无 DB 依赖（队列为内存实现）
- **依赖的库**：无外部运行时库，仅标准库

## 深入阅读

- ReviewResult JSON 解析（容错 LLM 输出）：`provider/vision.go:parseReviewJSON`
- 视频逐帧审核（取最坏帧）：`provider/claude.go:ClaudeProvider.AnalyzeVideo`
- applyThresholds 覆盖 LLM 决策：`review/aggregator.go:applyThresholds`
- Human Queue 接口定义（可替换为 DB/Kafka）：`queue/human_queue.go:Queue`
