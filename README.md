# AgentTrace

> 单二进制 AI Agent 调用链追踪工具。一行命令启动，零配置，本地追踪 AI Agent 的每次 LLM 调用、Token 花费和耗时。

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Python SDK](https://img.shields.io/badge/SDK-Python-3776AB?logo=python)](sdk/python/)

## 为什么需要 AgentTrace？

当你构建 AI Agent 应用时，每次调用可能涉及多次 LLM 请求、工具调用、检索操作。AgentTrace 帮你：

- **追踪调用链**：看到每个 Agent 执行了哪些 LLM 调用和工具调用
- **了解花费**：按 Trace、按 Model 实时统计 Token 消耗和费用
- **定位瓶颈**：时间轴视图展示每次调用的耗时
- **调试失败**：错误 Span 高亮显示，包含错误信息

所有数据存储在本地 SQLite，无需联网，不依赖外部服务。

## 快速开始

### 1. 启动 AgentTrace 服务

```bash
# 下载或构建二进制文件
go build -ldflags "-s -w" -o agenttrace.exe ./cmd/agenttrace

# 启动
.\agenttrace.exe -db agenttrace.db

# 浏览器打开
# http://localhost:8080
```

### 2. 接入 Python SDK

```bash
pip install -e sdk/python/
```

```python
import agenttrace
agenttrace.init("http://localhost:8080")

# 此后所有 OpenAI 调用自动被追踪
from openai import OpenAI
client = OpenAI()
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello, world!"}]
)
# → Trace + Span 自动发送到 AgentTrace
```

### 3. 生成 Demo 数据

```bash
go run ./cmd/seed
```

## API 文档

所有 API 返回 JSON，Cross-Origin Resource Sharing (CORS) 已启用。

### Traces

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/traces` | 获取 Trace 列表，参数：`limit`(默认50), `offset`(默认0) |
| `GET` | `/api/traces/:id` | 获取 Trace 详情（含所有 Span） |
| `POST` | `/api/traces` | 创建 Trace |
| `DELETE` | `/api/traces/:id` | 删除 Trace（级联删除所有 Span） |

**POST /api/traces** 请求体：

```json
{
  "id": "trace-001",
  "name": "Customer Support Agent",
  "session_id": "session-123",
  "user_id": "user-456",
  "tags": ["production", "customer-support"],
  "metadata": {}
}
```

### Spans

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/spans/:id` | 获取单个 Span 详情 |
| `POST` | `/api/spans` | 创建 Span（自动创建关联 Trace 如不存在） |

**POST /api/spans** 请求体：

```json
{
  "id": "span-001",
  "trace_id": "trace-001",
  "parent_span_id": "",
  "name": "chat.completions.create(gpt-4o)",
  "kind": "LLM",
  "status": "ok",
  "model": "gpt-4o",
  "provider": "openai",
  "input_json": "[{\"role\":\"user\",\"content\":\"Hello\"}]",
  "output_json": "{\"content\":\"Hi there!\"}",
  "prompt_tokens": 10,
  "completion_tokens": 5,
  "total_tokens": 15,
  "cost": 0.000075,
  "started_at": "2026-07-05T12:00:00Z",
  "ended_at": "2026-07-05T12:00:01.5Z",
  "error_message": "",
  "metadata": {}
}
```

**Span Kind 枚举：** `LLM` | `TOOL` | `RETRIEVAL` | `CHAIN` | `AGENT`

**Span Status 枚举：** `ok` | `error`

### Stats

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/stats` | 获取全局统计 |

响应示例：

```json
{
  "total_traces": 42,
  "total_spans": 156,
  "total_cost": 1.2345,
  "total_tokens": 125000,
  "error_count": 3
}
```

### Search

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/search?q=关键词` | 按 Trace 名称、Span 输入/输出内容搜索 |

### Health

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/health` | 健康检查 |

## 数据模型

```
Trace                            ← 一次完整的 Agent 运行
  └── Span (多个)                ← 一次 LLM 调用 / 工具调用 / 检索操作
       ├── input_json            ← 输入参数 (JSON)
       ├── output_json           ← 返回结果 (JSON)
       ├── tokens                ← Token 消耗 (prompt + completion)
       ├── cost                  ← 费用 (USD)
       ├── started_at / ended_at ← 时间戳
       └── metadata              ← 扩展字段

父子关系：通过 parent_span_id 表达多层嵌套的调用链
```

## 架构

```
cmd/agenttrace/        # 入口，CLI 参数解析，启动 HTTP
internal/tracer/       # Trace/Span 数据模型
internal/storage/      # SQLite CRUD（modernc.org/sqlite，零 CGO）
internal/api/          # REST API 层
internal/web/          # //go:embed 嵌入前端 dist
sdk/python/            # Python SDK（monkey-patch openai）
web/                   # React SPA（Deep Observatory 设计）
```

## 开发

```bash
# 构建
go build -ldflags "-s -w" -o agenttrace.exe ./cmd/agenttrace

# 前端开发（热重载）
cd web && npm run dev

# Go 后端开发（代理前端到 Vite dev server）
go run ./cmd/agenttrace -dev

# 测试
go test ./...

# 生成 demo 数据
go run ./cmd/seed

# 集成测试（10 个测试用例）
go run ./cmd/integration
```

## 技术栈

- **后端**: Go 1.21+, modernc.org/sqlite (纯 Go SQLite), net/http
- **前端**: React 18, React Router 6, Vite
- **SDK**: Python 3.8+ (monkey-patch openai), JS/TS (Proxy 拦截)

## License

MIT — 详见 [LICENSE](LICENSE)
