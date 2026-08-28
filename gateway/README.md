<p align="center">
  <img alt="Grok2API" src="./frontend/public/grok2api.png" width="720" />
</p>

<p align="center">
  <strong>面向 Grok Build、Grok Web 与 Grok Console 的多账号 API 网关</strong>
</p>

<p align="center">
  <a href="./backend/go.mod"><img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" /></a>
  <a href="./frontend/package.json"><img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=111827" /></a>
  <a href="https://github.com/chenyme/grok2api/actions/workflows/docker-publish.yml"><img alt="Docker" src="https://github.com/chenyme/grok2api/actions/workflows/docker-publish.yml/badge.svg" /></a>
</p>

> [!TIP]
> **个人新项目**<br>
> 推荐个人新项目 [DEEIX-AI：DEEIX-Chat 轻量化 AI 平台](https://github.com/DEEIX-AI/DEEIX-Chat)：企业级模型路由、对话、文件、工具、计费、身份和运维的一体化 AI 平台，全面且极致的低占用，空载运行时仅占用 34 MB。

> [!NOTE]
> 本项目仅供学习与研究交流。请务必遵循 Grok 的使用条款及当地法律法规，不得用于非法用途！

> [!NOTE]
> 开源项目欢迎大家支持二开和 PR，但请保留原作者标识和前端标识，尊重他人劳动成果～！

## 赞助商

> [希望赞助这个项目？](mailto:chenyme03@gmail.com)

<table>
<tr>
<td width="200" align="center" valign="middle"><a href="https://go.apimart.ai/gh-grok2api"><img src="frontend/public/sponner/api-mart.jpg" alt="APIMart" width="180"></a></td>
<td valign="middle">感谢 APIMart 赞助了本项目！APIMart 是专注 AI 图片/视频生成的低价 API 平台，GPT-Image-2 低至 $0.006/张，1 美元可出图 160+ 张。图片、视频一套异步 API 通吃，提交任务拿 ID、回调取结果，跑批万张不超时、换模型不改代码。按量付费、无月费，通过此 <a href="https://go.apimart.ai/gh-grok2api">注册链接</a> 注册即可开用。</td>
</tr>
<tr>
<td width="200" align="center" valign="middle"><a href="https://www.packyapi.com/register"><img src="frontend/public/sponner/packycode.png" alt="PackyCode" width="180"></a></td>
<td valign="middle">PackyCode 是稳定专业的 API 中转服务商，支持 Claude Code、Codex、Gemini 及多种国模，提供统一高速入口、全栈可观测、风控与弹性扩容。<a href="https://www.packyapi.com/register">点此注册</a>，轻松将大模型接入业务流程。</td>
</tr>
<tr>
<td width="200" align="center" valign="middle"><a href="https://github.com/DEEIX-AI/DEEIX-Chat"><img src="frontend/public/sponner/deeix-chat_deeix-ai.png" alt="DEEIX AI / DEEIX Chat" width="160"></a></td>
<td valign="middle">DEEIX-Chat 是一款开源可部署的 AI Chat 平台，面向需要长期、稳定、统一使用多模型能力的个人、团队与企业，将模型、对话、文件、工具调用与后台管理整合为一套可部署、可扩展的系统。点击 <a href="https://github.com/DEEIX-AI/DEEIX-Chat">此处</a> 开始部署！</td>
</tr>
<tr>
<td width="200" align="center" valign="middle"><a href="https://www.right.codes/register"><img src="frontend/public/sponner/rightcode.jpg" alt="RightCode" width="160"></a></td>
<td valign="middle">Right Code 是一个企业级 AI Agent 分发平台，主要提供稳定的 Claude Code、Codex、Gemini 等模型的中转服务。充值即可开票，企业、团队用户一对一对接。感谢 Right Code 提供的 Tokens 支持，点击 <a href="https://www.right.codes/register">此处</a> 注册并开始使用！</td>
</tr>
<tr>
<td width="200" align="center" valign="middle"><a href="https://api.fenno.ai/s/xCBS"><img src="frontend/public/sponner/fenno-ai.jpg" alt="FennoAI" width="160"></a></td>
<td valign="middle">FennoAI 面向企业研发团队和开发者提供企业级的高稳定、高性能 API 中转服务，兼容 OpenAI 与 Anthropic 协议，可接入 Codex、Claude Code、OpenCode 等主流 AI 编程工具。平台具备企业级稳定性，可支撑千亿 Token/日调用，以及境内外主体公对公结算与开票。Grok2API 用户通过<a href="https://api.fenno.ai/s/xCBS">专属链接</a>购买订阅，仅需 1.99 美元即可获得价值 50 美元的 Coding Plan 额度，邀请好友购买最高可获 20% 返佣。</td>
</tr>
<tr>
<td width="200" align="center" valign="middle"><a href="https://s.qiniu.com/RNNZFf"><img src="frontend/public/sponner/qiniu.jpg" alt="七牛云 AI" width="160"></a></td>
<td valign="middle">七牛云 AI 是七牛云（02567.HK）旗下企业级大模型 MaaS 平台，可一站式调用全球 150+ 主流模型，兼容主流模型厂商协议，覆盖文本、图像、音频、视频和文件处理等全模态能力，已服务超过 169 万企业及开发者用户。Grok2API 用户通过<a href="https://s.qiniu.com/RNNZFf">专属链接</a>注册，企业用户可免费领取 1200 万 Token，开发者可免费领取 300 万 Token。</td>
</tr>
</table>

Grok2API 是一个纯 Go 实现的 Grok API 网关。项目将 Grok Build OAuth、Grok Web SSO 与 Grok Console SSO 组织为独立账号池，对外提供 OpenAI 风格接口、Anthropic Messages 兼容接口，以及账号、模型、密钥、用量和代理管理后台。

## 功能概览

- **三 Provider**：`grok_build`、`grok_web` 与 `grok_console` 独立路由、额度和故障状态
- **标准接口**：Responses、Chat Completions、Images、异步 Videos、Anthropic Messages
- **多账号调度**：优先级、并发限制、额度门控、会话粘滞、冷却与故障切换
- **账号接入**：Device OAuth、OAuth JSON、SSO JSON、逐行 SSO Token
- **媒体能力**：图片生成、图片编辑、视频生成、图片本地归档与 URL/Base64 返回
- **基础设施**：SQLite/PostgreSQL、Memory/Redis、HTTP 与 SOCKS 代理池
- **安全边界**：AES-256-GCM 凭据加密、客户端密钥哈希、日志脱敏、SSRF 与传输上限
- **管理后台**：Dashboard、账号、模型、客户端密钥、请求审计、接口文档与热加载设置

## 架构

```mermaid
flowchart LR
    Client["API Client"] --> Gateway["Go Gateway"]
    Admin["React Admin"] --> Gateway

    Gateway --> Router["Model Router"]
    Router --> Build["Grok Build"]
    Router --> Web["Grok Web"]
    Router --> Console["Grok Console"]

    Build --> BuildPool["OAuth Account Pool"]
    Web --> WebPool["SSO Account Pool"]
    Console --> ConsolePool["SSO Account Pool"]
    Build --> Egress["Egress Pool"]
    Web --> Egress
    Console --> Egress

    Gateway --> Database["SQLite / PostgreSQL"]
    Gateway --> Runtime["Memory / Redis"]
    Gateway --> Media["Media Storage"]
```

## 快速部署

### Docker Compose

1. 准备配置：

```bash
git clone https://github.com/chenyme/grok2api.git
cd grok2api
cp config.example.yaml config.yaml
```

2. 生成并填写安全密钥：

```bash
openssl rand -hex 32
openssl rand -base64 32
```

```yaml
secrets:
  jwtSecret: "替换为 hex 随机值"
  credentialEncryptionKey: "替换为 Base64 随机密钥"

bootstrapAdmin:
  username: "admin"
  password: "替换为强密码"
```

3. 启动：

```bash
docker compose pull
docker compose up -d
```

访问 `http://127.0.0.1:8000`。

官方镜像已经包含前端构建产物，管理端与 API 由同一个 Go 服务提供。Compose 默认将 `config.yaml` 只读挂载到容器，并使用 `grok2api-data` 命名卷保存 SQLite 数据库和本地媒体。

常用命令：

```bash
docker compose logs -f grok2api
docker compose restart grok2api
docker compose down
```

### 源码运行

后端：

```bash
cp config.example.yaml config.yaml
cd backend
go run ./cmd/grok2api
```

前端开发服务器：

```bash
cd frontend
pnpm install
pnpm dev
```

前端默认运行于 `http://127.0.0.1:5173`，并将 API 请求代理到 `http://127.0.0.1:8000`。

## 首次使用

1. 使用 `bootstrapAdmin` 配置的管理员登录。
2. 在“上游账号”中接入 Grok Build、Grok Web 或 Grok Console 账号。
3. 等待本次额度和模型能力同步完成。
4. 在“模型管理”中确认对外模型名称与启用状态。
5. 在“客户端密钥”中创建 `g2a_` API Key。
6. 使用该密钥调用 `/v1/*`。

首次管理员创建后，建议修改管理员密码并从 `config.yaml` 删除 `bootstrapAdmin` 段。`credentialEncryptionKey` 必须长期保留，更换后已有凭据将无法解密。

## 账号来源

| Provider | 认证方式 | 主要能力 |
| :-- | :-- | :-- |
| Grok Build | Device OAuth、OAuth JSON | 原生 Responses、Chat、Messages、Billing、模型同步 |
| Grok Web | SSO JSON、逐行 SSO Token | Chat、Responses、Messages、图片、图片编辑、视频 |
| Grok Console | SSO JSON、逐行 SSO Token | 无状态 Responses、兼容 Chat 与 Messages |

Build refresh token 在续期时可能轮换。不要让 grok2api、官方 CLI、其他网关或独立客户端同时共用一份 Build 凭据，否则其中一个客户端可能消耗另一个客户端仍持有的 token；每个活跃客户端应分别授权，或在旧客户端停止使用后再迁移凭据。

Web 账号工具可接受条款、随机设置对应 20–40 岁的生日并启用 NSFW；已完成步骤会被记录，后续执行自动跳过。

旧 `reauthRequired` 账号的自动删除功能默认关闭；活跃推理租约和视频任务会受到保护。

> [!TIP]
> 从 Python 版本迁移时，可将 Grok Web SSO token 导出为 TXT 后在 **Grok Web** 中导入。旧版号池元数据和数据库不兼容。

Grok Build OAuth 支持按需续期。Grok Web 与 Grok Console 的 SSO 不可自动续期，凭据失效后账号会退出可用号池并等待重新授权。

Grok Web 与 Grok Console 均支持账号列表 JSON，也支持每行一个 Token 的快速导入。账号接入接口会等待本批账号的首次额度与模型能力同步完成后再返回结果。

管理端可复用 Web 账号的同一份 SSO 创建或更新对应的 Console 账号；同步按 Console 身份键幂等执行，不会改变已有 Web/Build 关联。

Grok Console 固定使用 `store: false`，不支持 `previous_response_id`、Response 查询/删除或 `/responses/compact`。多轮调用应像 Codex 无状态链路一样回放完整输入、工具调用和工具结果；网关不会为 Console 响应登记虚假的持久化归属。

## 模型

对外模型名称不带 Provider 前缀，例如 `grok-4.5`。内部上游路由使用 `Build/`、`Web/`、`Console/` 前缀区分实际来源；Grok Build 模型根据账号能力动态同步，请以管理端模型页或 `GET /v1/models` 为准。

升级时会原位迁移内部路由并保留路由主键、客户端密钥权限和旧名称别名。多个来源可以提供同一个对外模型名称；网关会按客户端权限、协议能力和账号可用性选择来源。带 Provider 前缀的名称仍可作为兼容入口，用于显式指定渠道。

| Model | Type | Minimum tier | Gateway surfaces |
| :-- | :-- | :-- | :-- |
| `grok-chat-fast` | Conversation | Basic | Chat Completions, Responses, Messages |
| `grok-chat-auto` | Conversation | Super | Chat Completions, Responses, Messages |
| `grok-chat-expert` | Conversation | Super | Chat Completions, Responses, Messages |
| `grok-chat-heavy` | Conversation | Heavy | Chat Completions, Responses, Messages |
| `grok-imagine-image-lite` | Image | Basic | Images Generations |
| `grok-imagine-image` | Image | Basic | Images Generations (`enable_pro=false`) |
| `grok-imagine-image-2.0` | Image | Basic | Images Generations (`enable_pro=true`) |
| `grok-imagine-image-edit` | Image Edit | Basic | Images Edits |
| `grok-imagine-video` | Video | Basic for 720p; Super for 480p | Videos |

Web Imagine generation maps `aspect_ratio` and `n` to the browser protocol. `size` remains an OpenAI-compatible aspect-ratio alias, while generation-only `resolution` and `quality` are ignored on Web routes because the upstream product is selected by the model name rather than by those Console-oriented controls.

Grok Web 内置模型：

| 模型 | 能力 | 最低等级 |
| :-- | :-- | :-- |
| `grok-chat-fast` | Chat / Responses / Messages | Basic |
| `grok-chat-auto` | Chat / Responses / Messages | Super |
| `grok-chat-expert` | Chat / Responses / Messages | Super |
| `grok-chat-heavy` | Chat / Responses / Messages | Heavy |
| `grok-imagine-image` | Fast 图片生成 | Basic |
| `grok-imagine-image-quality` | Quality 图片生成 | Super |
| `grok-imagine-image-edit` | 图片编辑 | Super |
| `grok-imagine-video` | 视频生成 | Super |

Grok Console 内置模型：

| 模型 | 能力 |
| :-- | :-- |
| `grok-4.3` | Responses / Chat / Messages |
| `grok-4.20-0309` | Responses / Chat / Messages |
| `grok-4.20-0309-reasoning` | Responses / Chat / Messages |
| `grok-4.20-0309-non-reasoning` | Responses / Chat / Messages |
| `grok-4.20-multi-agent-0309` | Responses / Chat / Messages |
| `grok-build-0.1` | Responses / Chat / Messages |

`grok-4.5` 不由 Grok Console Provider 注册；即使由 Web SSO 同步创建 Console 账号，该模型在 Console 中仍不可用。

Console 上游路由始终使用 `Console/` 内部前缀，不再根据启动顺序生成 `-console` 冲突后缀。升级产生的兼容别名不会出现在 `GET /v1/models`。

Responses 与 Messages 支持流式输出、工具、推理、多轮会话和压缩。稳定的客户端会话信号会被保留以维持 Grok Build 提示词缓存亲和性；缓存命中仍要求兼容的上游账号和未变化的提示词前缀。由当前网关生成且仍可解密的压缩摘要，即使会话或 `PromptCacheKey` 发生重映射也会被展开；外部或无法解密的摘要仍属于兼容边界。

同名模型会在当前可用来源中自动选路；来源选定后，账号故障切换只发生在该 Provider 的账号池内。

## API

除健康检查和公开图片外，所有 `/v1` 接口都需要客户端 API Key：

```http
Authorization: Bearer g2a_xxx_xxx
```

| 方法 | 路径 | 说明 |
| :-- | :-- | :-- |
| `GET` | `/healthz` | 存活检查 |
| `GET` | `/readyz` | 就绪检查 |
| `GET` | `/v1/models` | 当前可服务模型 |
| `POST` | `/v1/responses` | Responses JSON / SSE |
| `POST` | `/v1/responses/compact` | Responses compact |
| `GET` | `/v1/responses/{id}` | 查询 Response |
| `DELETE` | `/v1/responses/{id}` | 删除 Response |
| `POST` | `/v1/chat/completions` | Chat Completions JSON / SSE |
| `POST` | `/v1/messages` | Anthropic Messages JSON / SSE |
| `POST` | `/v1/images/generations` | 图片生成 |
| `POST` | `/v1/images/edits` | 图片编辑 |
| `GET` | `/v1/media/images/{id}` | 公开归档图片 |
| `POST` | `/v1/videos/generations` | 创建视频任务 |
| `GET` | `/v1/videos/{request_id}` | 查询视频任务 |

Responses 资源查询、删除和 compact 的实际可用性取决于目标模型所属 Provider；Grok Console 仅支持无状态 `POST /v1/responses`。

管理端登录后可在 `/docs` 查看当前 Base URL、可用模型以及 cURL、Python 和 JavaScript 示例。开发环境还可以在 `config.yaml` 设置 `server.swaggerEnabled: true`，通过 `/swagger/index.html` 查看公开 API 的 Swagger 文档；生产环境应保持关闭。

最小调用示例：

```bash
export GROK2API_API_KEY="g2a_xxx_xxx"

curl http://127.0.0.1:8000/v1/responses \
  -H "Authorization: Bearer $GROK2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-chat-auto",
    "input": "用三句话解释量子隧穿",
    "stream": true
  }'
```

## Egress and Cloudflare

Egress nodes are scoped to Build, Web, Console, or Web assets. The admin console supports:

- HTTP, HTTPS, SOCKS4/4A, SOCKS5/5H, and Resin
- Subscription and text/Base64 import
- Batch probes, filtering, deletion, assignment, and balancing
- Fallback per scope: none, direct, or a fixed node
- Proxy-pool mode without global cooldown after one connection failure
- Immediate recovery probes after fixed-proxy transport failures, with per-node coalescing and bounded waiting for fast retry
- Optional [Egress Quality Guard](./tools/egress-quality-guard/README.md) for active per-node model probes, guarded quarantine, and recovery; enable it with the built-in `quality-guard` Compose profile
- Give each sticky session its own fixed node (`proxyPool=false`). Do not merge several stickies into one node, or the guard can only quarantine the whole group

To enable the guard, add a `qualityGuard` section to `config.yaml`, then start
the profile. The main service creates and reuses a non-exportable system probe
identity automatically:

```yaml
qualityGuard:
  enabled: true
  model: "grok-4.5"
  # Optional: withhold thinking-model streams that have no reasoning.
  requestRetry:
    enabled: false
    maxAttempts: 6
    holdTimeout: 3s
    minOutputTokens: 32
    onExhausted: fail_closed # fail_open | fail_closed
```

`requestRetry` runs on the gateway request path and is independent of the sidecar. It is off by default. When enabled, a thinking-model stream with enough visible output and no reasoning is **not delivered**; another account is tried. If every attempt still has no reasoning, `onExhausted` either returns `503 quality_degraded` or delivers the last body. Image, video, tool, stored-response, and ForcedEgress probe requests are unchanged.

```bash
docker compose --profile quality-guard up -d --build
```

Existing preview deployments that still contain `clientKeyID` can upgrade
directly. The field is accepted for compatibility but ignored and can be
removed; any manually created probe key is intentionally left untouched.

After changing this configuration, run `docker compose --profile quality-guard restart grok2api egress-quality-guard` to reload the base settings; policy edits made in the admin page still hot-reload.

The normal `docker compose up -d` command does not start the guard or generate
probe traffic. The sidecar receives a narrowly scoped internal credential from
the main service and never stores or uses the administrator password. See the
linked guide before enabling automatic quarantine.

Resin usernames can contain `{account}`:

```text
socks5h://Default.{account}:RESIN_PROXY_TOKEN@resin:2260
```

The placeholder becomes a stable anonymous identity. Linked Web, Build, and Console accounts can share it; raw tokens and email addresses are not used.

## 配置与存储

根目录 `config.yaml` 保存启动配置：

| 分组 | 说明 |
| :-- | :-- |
| `server` | 监听地址、请求体上限、请求生命周期与 Swagger 开关 |
| `frontend` | 公开 API 地址与静态前端目录 |
| `database` | SQLite 或 PostgreSQL |
| `runtimeStore` | Memory 或 Redis |
| `auth` | 管理员 Token 与安全 Cookie |
| `secrets` | JWT 与凭据加密密钥 |
| `provider` | Build/Web/Console 上游默认配置 |
| `media` | 媒体存储驱动与路径 |

账号、模型、额度、审计、客户端密钥、媒体任务和运行设置始终保存在关系型数据库。Redis 用于限流、并发租约、粘滞路由、分布式锁、额度恢复事件和多实例设置通知。

When a fixed proxy enters cooldown after a transport failure, grok2api starts an independent connectivity probe immediately. Concurrent failures share one probe. A later request bound to that node waits for at most five seconds, reloads persisted node state after a healthy probe, and continues without waiting for the full cooldown. An unhealthy probe preserves the cooldown. Proxy-pool leases use fresh tunnels, so one rotating exit failure never cools the whole pool. See [Immediate egress failure probe and bounded retry](./backend/internal/infra/egress/FAILURE_RETRY.md) for the design and safety invariants.

推荐组合：

| 场景 | 数据库 | 运行态 | 媒体 |
| :-- | :-- | :-- | :-- |
| 本地或单实例 | SQLite | Memory | 本地目录 |
| 多实例 | PostgreSQL | Redis | 共享卷或实例亲和 |

Provider（包括 Console 上游地址与 User-Agent）、服务容量、批量任务并发、路由、媒体、审计和代理参数统一在管理端 `/settings` 修改，不需要直接编辑数据库；除页面明确标记“重启生效”的字段外均会热加载。导入同步、账号转换、数据同步和凭据刷新默认并发均为 `25`，可分别限制为 `1–50`，并支持随机启动延迟；多实例使用 Redis 时，分类上限和总上限均在集群范围内生效。

PostgreSQL credentials can be injected without storing them in `config.yaml`:

```bash
GROK2API_DATABASE_URL='postgresql://user:password@host:5432/grok2api?sslmode=require' docker compose up -d
```

A non-empty `GROK2API_DATABASE_URL` overrides `database.postgres.dsn` and automatically selects the `postgres` driver. An empty value is ignored. Supported URL schemes are `postgres://` and `postgresql://`; SQLAlchemy's `postgresql+asyncpg://` form is rejected with a migration hint. The application does not implicitly read the generic `DATABASE_URL`; platforms that provide it can map it explicitly with `GROK2API_DATABASE_URL: "${DATABASE_URL}"`. Database configuration precedence is built-in defaults, `config.yaml`, then `GROK2API_DATABASE_URL`. The current CLI has no database override.

### Client IPs behind a reverse proxy

Request audits record the normalized client IPv4 or IPv6 address. Direct deployments need no extra configuration. Behind Nginx or another reverse proxy, configure both sides:

1. Forward the standard client IP headers from the proxy:

```nginx
location / {
    proxy_pass http://127.0.0.1:8000;

    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

2. Trust only the proxy address or its isolated network in `config.yaml`:

```yaml
server:
  trustedProxies:
    - "127.0.0.1"
```

With Docker, the peer seen by grok2api may be the bridge gateway or another container rather than `127.0.0.1`. Inspect the Compose network before configuring it:

```bash
docker network inspect grok2api_default \
  --format '{{(index .IPAM.Config 0).Subnet}}'
```

For example, an isolated network reported as `172.20.0.0/16` can be configured as a trusted proxy CIDR. Never use `0.0.0.0/0` or `::/0`; grok2api rejects unrestricted trusted-proxy ranges. Without `trustedProxies`, forwarded headers are ignored and audits contain the direct TCP peer address, preventing clients from spoofing `X-Forwarded-For`.

If Cloudflare is in front of Nginx, configure Nginx's real-IP module with `CF-Connecting-IP` and Cloudflare's official proxy ranges first. Do not trust `CF-Connecting-IP` from arbitrary peers. Restart grok2api after changing `server.trustedProxies`; reload Nginx after changing its configuration.

Important optional settings:

- `audit.ledgerMode`: `observe` reports ledger faults; `enforce` can pause new inference to protect billing integrity.
- `routing.accountIsolatedConnections`: partitions outbound TCP/HTTP pools by account for external L4 or connection-hash load balancers. It is off by default because it increases connections, TLS handshakes, memory, and file-descriptor usage.
- `routing.segmentedSelectorEnabled`: enabled by default for pools with at least 3,000 eligible accounts; bounds dynamic concurrency reads while retaining quota/tier priorities, sticky sessions, full-planner fallback, and atomic guards.
- `routing.autoAssignMaxNodeShare` / `routing.autoAssignMaxMigrationShare`: optional large-pool guards. `0` (default) keeps the historical unbounded first-pass evacuation and the existing 200-move ceiling for capacity/rebalance repair. Set `0.05`–`1` only when a quarantined node would otherwise dump thousands of auto accounts onto the last healthy exits. `GROK2API_AUTO_ASSIGN_MAX_NODE_SHARE` and `GROK2API_AUTO_ASSIGN_MAX_MIGRATION_SHARE` override the YAML when set.
- Build response-header timeout and exact-match 403 invalidation rules are hot-reloadable.
- **Sync latest version** applies the validated Grok Build client version and User-Agent.

## 生产部署

- 使用 HTTPS，并设置 `auth.secureCookies: true`
- 保持 `server.swaggerEnabled: false`
- 多实例部署使用 PostgreSQL 与 Redis
- 本地媒体目录在多实例下必须使用共享卷或实例亲和
- 持久化备份 `config.yaml`、关系型数据库和媒体目录
- 不要将 OAuth、SSO、Cloudflare Cookie 或账号导出文件提交到 Git
- 对外暴露前建议配置反向代理、访问日志和基础网络防护

## 开发

后端：

```bash
cd backend
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/grok2api
```

前端：

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm lint
pnpm build
```

## 进一步阅读

- [后端说明](./backend/README.md)
- [前端说明](./frontend/README.md)
