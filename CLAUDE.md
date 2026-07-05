# AgentTrace

单二进制 AI Agent 调用链追踪工具。MIT 开源，Go + SQLite + React。

## 核心价值
> 一行命令启动，零配置，本地追踪 AI Agent 的每次 LLM 调用、Token 花费和耗时。

## 开发命令

```bash
# 构建
go build -o agenttrace ./cmd/agenttrace

# 开发模式（实时重载前端）
cd web && npm run dev          # 前端开发服务器 :5173
go run ./cmd/agenttrace -dev   # Go 后端 :8080，代理前端请求

# 测试
go test ./...                   # 全部测试
go test ./internal/tracer/...   # 单个 package

# 单二进制打包（嵌入前端 dist）
cd web && npm run build
go build -o agenttrace ./cmd/agenttrace

# 交叉编译
GOOS=linux GOARCH=amd64 go build -o agenttrace-linux ./cmd/agenttrace
GOOS=darwin GOARCH=arm64 go build -o agenttrace-darwin ./cmd/agenttrace
GOOS=windows GOARCH=amd64 go build -o agenttrace.exe ./cmd/agenttrace
```

## 架构

```
cmd/agenttrace/        # 入口 main.go，解析 CLI 参数，启动 HTTP 服务
internal/tracer/       # 核心追踪逻辑：Span/Trace 数据模型 + SDK 协议定义
internal/storage/      # SQLite 持久化，纯 Go 驱动（modernc.org/sqlite），零 CGO
internal/api/          # HTTP API 层，JSON REST 接口
internal/web/          # 嵌入式前端资源，//go:embed 打包 React dist
sdk/python/            # Python SDK：monkey patch openai 模块
sdk/js/                # JS/TS SDK：Proxy 拦截 fetch/axios
web/                   # React SPA，Trace 树形图 + 时间轴 + 开销面板
```

## 数据模型

```
Trace                            ← 一次完整的 Agent 运行
  └── Span (多次)                ← 一次 LLM 调用 / 工具调用 / 检索操作
       ├── input                 ← 输入参数
       ├── output                ← 返回结果
       ├── tokens (prompt+completion)  ← Token 消耗
       ├── cost                  ← 费用 ($)
       ├── started_at / ended_at ← 时间戳
       └── metadata              ← 扩展字段 (model, provider, tags)

三表结构：traces / spans / events (多级 span 嵌套用 parent_span_id 表达)
```

## 关键设计决策

- **不做 Prompt 管理、不做 LLM 评测、不做 Playground** —— 只做追踪+开销+耗时
- **兼容 OpenTelemetry 语义规范** —— span 属性名对齐 otel gen_ai 命名
- **SDK 走 HTTP 上报** —— 不依赖 gRPC，SDK 批量 POST JSON 到本地 AgentTrace
- **纯 Go SQLite (modernc.org/sqlite)** —— 零 CGO 依赖，交叉编译无痛
- **Go 1.16+ embed** —— 前端静态文件编译进二进制，单文件分发
