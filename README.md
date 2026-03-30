# Market Project

Overnight MVP for Eastmoney AI Market Copilot.

## Quick Start

先开三个终端。

本地开发前提：

- `apps/backend/.env` 里要使用本机地址，不要保留 Docker 主机名
- 当前本地默认值应该是：
  - `AI_SERVICE_URL=http://localhost:8081`
  - `DATABASE_URL=postgres://postgres:postgres@localhost:5432/market_copilot?sslmode=disable`
  - `EMBEDDED_POSTGRES=true`
  - `REDIS_ADDR=127.0.0.1:6379`
- `redis-server` 需要先可用，`redis-cli ping` 应返回 `PONG`
- 如果本机没有 PostgreSQL，直接用 embedded PostgreSQL 即可；`api-go` 和 `ai-go` 现在会自动复用同一个 embedded 实例

终端 1，启动 backend：

```bash
cd /mnt/d/project-go/Market-project/apps/backend
cp .env.example .env
go run ./cmd/server
```

如果看到 `postgres unavailable, starting embedded postgres`，这是正常的首次启动行为。

终端 2，启动 ai-server：

```bash
cd /mnt/d/project-go/Market-project/apps/backend
cp .env.example .env
go run ./cmd/ai-server
```

如果 backend 已经先启动，`ai-server` 会直接复用刚启动的 embedded PostgreSQL。

终端 3，启动 frontend：

```bash
cd /mnt/d/project-go/Market-project/apps/frontend
cp .env.example .env
pnpm install
pnpm dev --host 127.0.0.1 --port 5173
```

打开：

- frontend: `http://127.0.0.1:5173`
- api health: `http://127.0.0.1:8080/health`
- ai health: `http://127.0.0.1:8081/health`

本地自检：

- `curl http://127.0.0.1:8080/health`
- `curl http://127.0.0.1:8081/health`
- 如果前端出现 `ERR_CONNECTION_REFUSED`，先检查这两个 health
- 如果前端出现 provider/billing 为空，先确认登录态和 backend/ai-go 都已经起来

默认测试账号：

- email: `demo@example.com`
- password: `demo123456`

## What Works Now

- Go backend with JWT auth
- Embedded PostgreSQL fallback for local dev
- Eastmoney sync for 3 default symbols
- Eastmoney full symbol catalog sync into PostgreSQL on startup, then once every 24 hours
- OHLC storage and query
- AI copilot endpoint with OpenAI-compatible config
- Copilot SSE streaming endpoint with non-stream fallback
- Copilot panel supports manual stop while streaming
- Copilot streaming now exposes stage hints like loading OHLC, checking allowance, and generating answer
- Copilot panel renders a lightweight stage timeline during streaming
- Multi-turn copilot history passed through to the model
- Copilot sessions are split by day and kept for the latest 7 days
- Recent sessions support summary, favorite pinning, and collapsible display
- Session list now shows a title-style summary and supports favorites-only filtering
- Heuristic fallback when model config is missing
- React + Ant Design frontend shell
- Login page and protected dashboard
- Symbol list, chart panel, chart drag range selection, AI result panel

## Agent And Skills

This repo now defines a repo-level agent workflow instead of binding behavior to one model vendor.

Supported model-provider direction:

- OpenAI
- DeepSeek
- Gemini

The rule is:

- models may change
- grounded research flow should stay stable
- news or public context must go through controlled tool paths instead of free-form model hallucination

Repo operating instructions live in:

- [AGENT.md](/mnt/d/project-go/market-project/AGENT.md)
- [docs/skills/market-news-research/SKILL.md](/mnt/d/project-go/market-project/docs/skills/market-news-research/SKILL.md)
- [docs/skills/chart-copilot/SKILL.md](/mnt/d/project-go/market-project/docs/skills/chart-copilot/SKILL.md)

Recommended future execution path:

1. backend research adapter fetches symbol-range news
2. adapter normalizes source metadata
3. chart copilot receives OHLC indicators plus normalized news evidence
4. selected provider model generates final structured answer

## Default Demo Account

- email: `demo@example.com`
- password: `demo123456`

## Backend

```bash
cd apps/backend
cp .env.example .env
go run ./cmd/server
```

Backend default URL:

- `http://localhost:8080`
- `AI_SERVICE_URL`: api-go forwards `/ai/*` and `/copilot/*` to this service, default `http://localhost:8081`

## AI Service

```bash
cd apps/backend
cp .env.example .env
go run ./cmd/ai-server
```

AI service default URL:

- `http://localhost:8081`
- `AI_PORT`: ai-go listen port, default `8081`

Notes:

- If local PostgreSQL is not running, backend will try to start embedded PostgreSQL automatically.
- First boot may take longer because embedded PostgreSQL may initialize or download binaries.
- If you already have PostgreSQL, you can point `.env` to your own PG and avoid embedded startup time.

## Frontend

```bash
cd apps/frontend
cp .env.example .env
pnpm install
pnpm dev --host 127.0.0.1 --port 5173
```

Frontend default URL:

- `http://localhost:5173`

Frontend service split:

- `VITE_API_BASE_URL`: main backend base URL for auth, symbols, billing
- `VITE_COPILOT_API_BASE_URL`: optional AI/copilot service base URL
- if `VITE_COPILOT_API_BASE_URL` is unset, copilot requests fall back to `VITE_API_BASE_URL`

## Redis

当前已经接入 Redis，用于：

- `cache:symbols:list`
- `cache:ohlc:{symbol}:{start}:{end}`
- `cache:ai:providers`
- `cache:copilot:sessions:{userId}:{symbol}`
- `cache:copilot:messages:{userId}:{sessionId}`
- `cache:billing:user:{userId}`
- `rate:ai:user:{userId}`
- `rate:ai:ip:{ip}`
- `auth:token:blacklist:{jti}`

当前计费规则：

- 三档会员：`starter` / `active` / `pro`
- 会员购买后获得 30 天有效期的每日额度
- AI 请求优先消耗每日额度，不够时再扣免费 `credits`
- 兑换码只增加免费 `credits`，不会改会员状态
- 支付入口当前已隐藏，先通过兑换码运营会员
- 开发环境预置兑换码：
  - `WELCOME100`：增加 10000 免费 `credits`
  - `STARTER30`：激活 Starter，30 天，每日 10000
  - `ACTIVE30`：激活 Active，30 天，每日 40000
  - `PRO30`：激活 Pro，30 天，每日 140000

后台能力：

- `ADMIN_EMAILS` 白名单邮箱登录后会带 `is_admin=true`
- 前端提供 `/admin` 最小后台页
- 当前可做：
  - 生成会员兑换码
  - 生成免费 `credits` 兑换码
  - 批量生成兑换码
  - 设置兑换码过期时间
  - 禁用兑换码
  - 查看最近兑换码
  - 查看最近领取记录
  - 查看最近管理员操作记录
  - 查看最近用户和会员快照
  - 查看单个用户的会员、额度、兑换记录和最近 AI 用量
  - 直接给用户补免费 `credits` 或直接发会员
  - 按关键字、奖励类型、状态筛选后台数据
  - 批量结果一键复制，或导出为 `TXT/CSV`

支付相关接口：

- `POST /billing/recharge-orders`

## Symbol Catalog

现在 PostgreSQL 里分两层股票数据：

- `symbols`
  - 当前页面里已经添加到列表的股票
- `symbol_catalog`
  - 从东方财富同步下来的股票主数据
  - `api-go` 启动时会全量同步一次
  - 之后每 24 小时自动再同步一次，用来覆盖新上市股票和名称变更

当前前端仍然保留“输入代码添加”的模式，不会直接把全市场股票全部显示到左侧列表。

后端额外提供：

- `GET /symbols/search?q=002173`
- `GET /symbols/search?q=创新医疗`

用于按代码或名称搜索 `symbol_catalog`。
  - 创建待支付订单
- `POST /billing/recharge-orders/:id/mock-pay`
  - 模拟支付宝扫码成功并激活会员
- `POST /payments/alipay/mock/notify?order_id=...`
  - mock 支付宝服务端回调入口

默认配置：

- `REDIS_ENABLED=true`
- `REDIS_ADDR=127.0.0.1:6379`
- `REDIS_PASSWORD=`
- `REDIS_DB=0`
- `AI_USER_RATE_LIMIT=20`
- `AI_IP_RATE_LIMIT=60`

本地检查：

```bash
redis-cli ping
redis-cli --scan --pattern 'cache:*'
redis-cli --scan --pattern 'rate:ai:*'
redis-cli --scan --pattern 'auth:token:blacklist:*'
```

## Build Checks

```bash
cd apps/backend && go build ./...
cd apps/frontend && pnpm build
```

## Local Compose

```bash
cp apps/backend/.env.example apps/backend/.env
cp apps/frontend/.env.example apps/frontend/.env
docker compose up --build
```

Open:

- frontend: `http://localhost:4173`
- api-go: `http://localhost:8080/health`
- ai-go: `http://localhost:8081/health`

## Kubernetes

如果你不想再用宿主机手工起 `go run`，现在可以直接走 `minikube + kubectl`。

Minikube 清单位于：

- `deploy/minikube/01-base.yaml`
- `deploy/minikube/02-infra.yaml`
- `deploy/minikube/03-apps.yaml`

这 3 个文件分别负责：

- `01-base.yaml`
  - namespace
  - ConfigMap
  - Secret
- `02-infra.yaml`
  - PostgreSQL
  - Redis
  - PVC
- `03-apps.yaml`
  - `api-go`
  - `ai-go`
  - `frontend`
  - ingress

启动步骤：

```bash
minikube start --driver=docker
minikube addons enable ingress
```

构建镜像到 minikube：

```bash
docker build -t market-project/backend:minikube apps/backend
docker build \
  --build-arg VITE_API_BASE_URL=/api \
  --build-arg VITE_COPILOT_API_BASE_URL=/api \
  -t market-project/frontend:minikube \
  apps/frontend
minikube image load market-project/backend:minikube
minikube image load market-project/frontend:minikube
```

部署：

```bash
kubectl apply -f deploy/minikube/01-base.yaml
kubectl apply -f deploy/minikube/02-infra.yaml
kubectl apply -f deploy/minikube/03-apps.yaml
kubectl -n market-project get pods
```

查看入口：

```bash
minikube tunnel
kubectl -n ingress-nginx get svc ingress-nginx-controller
```

浏览器访问：

- `http://127.0.0.1.nip.io`

说明：

- 前端在 k8s 下统一请求 `/api`
- ingress 会把 `/api/*` 转发给 `api-go`
- `api-go` 继续代理 AI 路由到 `ai-go`
- 所以浏览器只需要一个入口，不需要知道集群里的 service 名

建议：

- 当前 `minikube` 清单里 PostgreSQL 和 Redis 是集群内单实例，只适合本地验证
- 真正上线到正式 k8s 时，PostgreSQL 和 Redis 仍然建议换托管服务

## Why Backend May Start Slowly

- `go run` 首次会编译 Go 依赖，第一次本来就会慢一点。
- 如果没有本地 PostgreSQL，项目会走 embedded PostgreSQL，本身启动就比普通 API 慢。
- embedded PostgreSQL 首次可能还要准备二进制，这一步最慢。

## Troubleshooting

如果 backend 卡住很久，先看这几个点：

```bash
cd /mnt/d/project-go/Market-project/apps/backend
go build ./...
go run ./cmd/server
```

如果你现在是双进程启动：

```bash
cd /mnt/d/project-go/Market-project/apps/backend
go run ./cmd/ai-server
go run ./cmd/server
```

注意：

- 第一次本地启动时，建议先起 `ai-server`，让 embedded PostgreSQL 初始化完成，再起 `server`
- 如果本机已经有 PostgreSQL，就直接配置 `DATABASE_URL`，避免两个进程同时抢 embedded PostgreSQL 初始化

如果 frontend 起不来：

```bash
cd /mnt/d/project-go/Market-project/apps/frontend
pnpm install
pnpm dev --host 127.0.0.1 --port 5173
```

如果 5173 端口被占用，可以改端口：

```bash
pnpm dev --host 127.0.0.1 --port 5174
```

如果你想确认 backend 是否正常：

```bash
curl http://127.0.0.1:8080/health
```

如果你想确认登录链路：

```bash
curl -X POST http://127.0.0.1:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@example.com","password":"demo123456"}'
```

## Key Endpoints

- `GET /health`
- `POST /auth/login`
- `GET /symbols`
- `GET /symbols/:symbol/ohlc`
- `POST /copilot/query`

## Current Default Tracked Symbols

- `600519`
- `000001`
- `300750`

## Current Gaps

- chart selection is already draggable, but it is still a lightweight overlay instead of a full professional chart annotation tool
- AI output is heuristic if no model env is configured
- no news and no realtime by design

## Next Recommended Work

- 把会话保存时连同当次技术快照一起落库
- 在 AI 面板固定显示支撑位、压力位、风险位
- 补更专业的图表交互，比如左右平移和更多指标开关
