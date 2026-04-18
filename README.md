# adortb-creative-review

> adortb 平台的 LLM 素材审核服务，通过 Claude / OpenAI 对广告文案、图片、视频帧和着陆页进行并行自动审核，输出风险评分和审核决策，不合格素材推送人工队列。

## 架构定位

```
┌─────────────────────────────────────────────────────────────────┐
│                      adortb 平台整体架构                         │
│                                                                  │
│  广告主提交素材                                                  │
│       │ POST /v1/review                                         │
│       ▼                                                         │
│  ★ adortb-creative-review (Creative Review Service)            │
│       │                                                         │
│  ┌────┼──────────────────────────────────────────────┐          │
│  │    ▼     [Aggregator] 并行审核协调器               │          │
│  │    ├──► [TextReviewer]    文案审核（LLM）           │          │
│  │    ├──► [ImageReviewer]   图片审核（Vision）        │          │
│  │    ├──► [VideoReviewer]   视频帧审核（逐帧）        │          │
│  │    └──► [LandingScanner]  着陆页内容扫描            │          │
│  │         ↓                                          │          │
│  │    [Policy] 阈值判断 → pass/warn/needs_human/reject│          │
│  │         ↓ needs_human                              │          │
│  │    [HumanQueue] 人工审核队列                        │          │
│  └─────────────────────────────────────────────────┘          │
│                                                                  │
│  Provider: Claude 3.5 Sonnet / GPT-4o / Mock                   │
└─────────────────────────────────────────────────────────────────┘
```

Creative Review 是平台**内容合规**的关键环节，在素材上线前自动过滤违规内容，降低人工审核压力。

## 目录结构

```
adortb-creative-review/
├── go.mod                          # Go 1.25.3，无外部 DB 依赖
├── cmd/creative-review/
│   └── main.go                     # 主程序：Provider 选择、服务启动（端口 8104）
├── migrations/                     # 审核记录持久化（可选）
└── internal/
    ├── api/
    │   └── handler.go              # HTTP 路由：/v1/review, /v1/human-queue, /v1/policy
    ├── review/
    │   ├── aggregator.go           # 并行协调器（goroutine per reviewer，取最严决策）
    │   ├── text_reviewer.go        # 文案审核（headline/description/landing_url）
    │   ├── image_reviewer.go       # 图片审核（Vision API）
    │   ├── video_reviewer.go       # 视频审核（逐帧 → 合并最坏结果）
    │   └── landing_scanner.go      # 着陆页扫描（URL 内容 + 关键词检测）
    ├── provider/
    │   ├── interface.go            # LLMProvider 接口定义
    │   ├── claude.go               # Anthropic Claude 3.5 Sonnet Provider
    │   ├── openai.go               # OpenAI GPT-4o Provider
    │   ├── vision.go               # 视觉分析工具函数
    │   └── mock.go                 # Mock Provider（测试/开发）
    ├── rules/
    │   ├── policy.go               # Policy：禁用词列表 + 决策阈值（线程安全动态更新）
    │   └── prompt_library.go       # PromptLibrary：各场景审核 Prompt 模板
    ├── queue/
    │   └── human_queue.go          # HumanQueue：人工审核队列（内存实现）
    ├── client/                     # Go 客户端
    └── metrics/
        └── metrics.go              # Prometheus 指标
```

## 快速开始

### 环境要求

- Go 1.25.3
- LLM API Key（不设则使用 Mock Provider）

```bash
export PATH="$HOME/.goenv/versions/1.25.3/bin:$PATH"
```

### 运行服务

```bash
cd adortb-creative-review

# 选择 LLM Provider（优先级：OPENAI > ANTHROPIC > Mock）
export ANTHROPIC_API_KEY="sk-ant-..."
# 或
export OPENAI_API_KEY="sk-..."

export PORT=8104

go run cmd/creative-review/main.go
```

### 运行测试

```bash
go test ./... -cover -race
```

## HTTP API

### POST /v1/review — 同步审核

提交素材进行实时 LLM 审核（文案必填，图片/视频/着陆页可选）。

**请求体**：

```json
{
  "creative_id": 12345,
  "headline": "限时特惠，手机好价！",
  "description": "正品行货，全国联保",
  "image_url": "https://cdn.example.com/ad-banner.jpg",
  "video_url": "https://cdn.example.com/ad-video.mp4",
  "video_frame_urls": [
    "https://cdn.example.com/frame-0.jpg",
    "https://cdn.example.com/frame-1.jpg"
  ],
  "landing_url": "https://shop.example.com/product/123",
  "category": "electronics"
}
```

**响应**：

```json
{
  "creative_id": 12345,
  "decision": "pass",
  "risk_score": 0.12,
  "categories": ["electronics"],
  "reasons": [],
  "total_tokens": 342,
  "reviewed_at": "2024-06-15T10:30:00Z"
}
```

**决策类型**：

| 决策 | 含义 |
|------|------|
| `pass` | 审核通过，可投放 |
| `warn` | 存在轻微风险，建议人工复核 |
| `needs_human` | 需要人工审核 |
| `reject` | 审核拒绝，不可投放 |

### POST /v1/review/async — 异步审核

提交素材后立即返回任务 ID，审核完成后可查询结果。

### GET /v1/reviews/{id} — 查询审核结果

### 人工队列

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/human-queue` | 查看待人工审核的素材列表 |
| POST | `/v1/human-queue/{id}/resolve` | 人工标注审核结果 |

### 策略管理

#### GET /v1/policy — 查询当前策略

#### PUT /v1/policy — 动态更新策略

```json
{
  "banned_keywords": ["casino", "xxx", "hack"],
  "warn_threshold": 0.3,
  "reject_threshold": 0.8,
  "human_review_threshold": 0.5
}
```

## 配置说明

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `ANTHROPIC_API_KEY` | — | Claude API Key |
| `OPENAI_API_KEY` | — | OpenAI API Key（优先于 Claude） |
| `PORT` | `8104` | 监听端口 |

## 审核流程

```
POST /v1/review
    │
    ▼
Policy.ContainsBannedKeyword()  快速粗筛（无需 LLM）
    │ 通过
    ▼
Aggregator.Review()  并发启动多个审核任务
    ├─ goroutine: TextReviewer.Review()
    ├─ goroutine: ImageReviewer.Review()（有 image_url 时）
    ├─ goroutine: VideoReviewer.Review()（有 video_url 时）
    └─ goroutine: LandingScanner.Scan()（有 landing_url 时）
    │
    ▼
applyThresholds()  按 Policy 阈值调整各子任务决策
mergeResultWorst() 取最严重的决策作为最终结果
    │
    ▼
final_decision == needs_human → 推送 HumanQueue
    │
    ▼
返回 AggregatedResult（decision/risk_score/reasons）
```

## 性能设计

| 机制 | 说明 |
|------|------|
| 并行审核 | text/image/video/landing 四路并发，总时延 = max(单任务) |
| 快速粗筛 | 禁用词匹配在 LLM 调用前执行，立即拒绝明显违规 |
| 写超时 | HTTP WriteTimeout=120s，支持慢速 LLM 响应 |
| Mock Provider | 无需 API Key 即可本地开发调试 |

## 相关项目

| 项目 | 说明 |
|------|------|
| [adortb-brand-safety](https://github.com/adortb/adortb-brand-safety) | 投放时页面安全检查 |
| [adortb-adx](https://github.com/adortb/adortb-adx) | 广告竞价引擎 |
| [adortb-infra](https://github.com/adortb/adortb-infra) | 基础设施 |
