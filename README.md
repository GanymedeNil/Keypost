# Digital Fulfillment

**Lemon Squeezy 数字交付（Digital Fulfillment）的自托管替代品，数据来自 Stripe，接口与 Lemon Squeezy 保持一致。**

Lemon Squeezy 将于 **2027-03-31** 停止处理交易。官方推荐迁移到 Stripe Managed Payments，但它**不提供** License Key、文件和下载链接的交付，
历史订单、Key、文件也不会自动迁移。本项目补上这一层：

| 能力 | 说明 |
| --- | --- |
| **LS 兼容 License API** | `POST /v1/licenses/activate \| validate \| deactivate`，请求/响应/错误文案/限流与 LS 一致。老版本客户端只需改 base URL |
| **LS 兼容 JSON:API** | `/v1/*`：stores、customers、products、variants、prices、files、orders、order-items、subscriptions、subscription-invoices/items、license-keys、license-key-instances、webhooks、checkouts；`page[]`/`filter[]`/`include`/`sort`、嵌套路由、限流头、测试模式 |
| **LS 兼容出站 Webhook** | 16 种事件，同名、同 payload 形状、`X-Event-Name` + `X-Signature`（HMAC-SHA256 hex）、`meta.custom_data`、事件顺序一致、失败重试（5s/25s/125s）、后台可重放 |
| **Stripe 履约** | 验签 Webhook → 幂等处理 `checkout.session.completed`、`invoice.paid`、`customer.subscription.*`、`charge.refunded`…，生成订单、订阅、License Key、发收据邮件 |
| **客户页面** | 魔法链接登录 My Orders、订单详情（License Key、文件下载、链接）、签名下载（1 小时有效、每文件每 IP 每天 10 次）、一键进入 Stripe 账单门户 |
| **数据迁移** | 从 LS API 或离线归档导入全部历史（**保留原 ID、License Key 字符串、实例 UUID、文件本体**），导入 LS→Stripe 客户映射表，自动匹配，缺口报告 |
| **管理后台** | Next.js：仪表盘、订单（退款/补发收据/导出 CSV）、客户、订阅、License Key、商品/变体/价格/文件、Webhook 投递日志、Stripe 事件重放、迁移中心、API Key、成员、审计日志、设置；中英双语；**支持「使用 Stripe 登录」** |

## 架构

```
 购买者 ──► Stripe Checkout ──► Stripe ──webhook──► /webhooks/stripe ─┐
 你的应用 ─► /v1/licenses/*  (License API, 无需鉴权)                   │
 你的服务端 ─► /v1/*  (JSON:API, Bearer Key)                           ▼
                                                            ┌──────────────────┐   出站 Webhook
 浏览器 ─► Next.js (:3000) ──rewrites──────────────────────►│  Go API (:8080)  │──► 你的业务系统
            /admin  管理后台                                 │  履约 · 授权 · 导入 │
            /my-orders  客户页面                             └───────┬──────────┘
                                                              PostgreSQL / SQLite   本地磁盘 / S3
```

- **后端**：Go（标准库 `net/http` + GORM），PostgreSQL（推荐）或 SQLite；无第三方 Stripe SDK，内置轻量客户端并**同时兼容 basil 前后的响应形状**。
- **前端**：Next.js App Router + Tailwind v4。同源反代，浏览器只访问 `:3000`。

详见 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)、[docs/API_COMPATIBILITY.md](docs/API_COMPATIBILITY.md)、[docs/MIGRATION.md](docs/MIGRATION.md)、[docs/RESEARCH_LEMONSQUEEZY.md](docs/RESEARCH_LEMONSQUEEZY.md)。

## 快速开始

### Docker Compose（生产形态）

```bash
cp .env.example .env          # 填 PUBLIC_URL / APP_SECRET / POSTGRES_PASSWORD / STRIPE_* …
docker compose up -d --build
docker compose logs backend | grep setup_token   # 首次启动：打开 /admin 用该令牌创建 owner
```

或在 `.env` 设置 `ADMIN_EMAIL` / `ADMIN_PASSWORD` 直接创建 owner。把 TLS 反向代理指到 `:3000`（API 路径 `/v1`、`/download`、`/webhooks` 可直连后端 `:8080` 以减少一跳）。

### 本地开发

```bash
make dev-backend     # Go API :8080（SQLite ./backend/data）
make dev-frontend    # Next.js :3000（代理到 :8080）
make test            # 后端测试（SQLite 内存库 + -race）
make test-pg         # 同一套测试跑在真实 PostgreSQL 16 上
```

## 接入 Stripe（5 步）

1. **密钥**：后台 *设置 → Stripe* 填 `Secret key`（建议 Restricted Key：Checkout Sessions、Customers、Subscriptions、Invoices、Charges、Refunds、Billing Portal、Products/Prices 读写，Webhook Endpoints 写）。
2. **Webhook**：*Stripe* 页点「创建 / 更新 Webhook Endpoint」，自动在 Stripe 建好 `PUBLIC_URL/webhooks/stripe` 并保存签名密钥（也可手动配置，事件见 `docs/ADMIN_API.md` §4）。
3. **目录**：*Stripe → 目录* 一键把 Stripe Products/Prices 导入为商品/变体/价格（或在变体上手动「关联 Stripe」）。迁移场景下 LS 的每个 variant 对应一个 Stripe product。
4. **发货规则**：在变体上勾选「生成 License Key」、设置激活上限/有效期，上传文件，加外部链接（支持 `[license_key] [order_id] [email] …` 变量）。
5. **下单入口**：商品的 `buy_now_url`（`/checkout/buy/{variant_slug}`）或 `POST /v1/checkouts`（与 LS 同参：`checkout_data.email/custom`、`custom_price`、`product_options.redirect_url`），自动创建 Stripe Checkout Session。付款后回到 `/checkout/success`，直接跳到带签名的订单页（或你配置的 `redirect_url`）。

> **Managed Payments**：设置里打开「Managed Payments」，创建 Checkout Session 时会带 `managed_payments[enabled]=true`（需 Stripe 账户已开通，且按 Stripe 要求使用对应的 API 版本）。

## 从 Lemon Squeezy 迁出

1. 后台 *导入* → 填 LS API Key（只读即可）→ 先 **Dry run** 看数量 → 正式导入（含文件下载）。也支持离线 ZIP 与订单 CSV。
2. 上传 LS 迁移工具给出的「LS 客户 ID → Stripe 客户 ID」映射表；对订阅、商品同理。其余用「自动匹配」按邮箱/名称匹配，*下载缺口报告* 处理剩余项。
3. 把应用里的 `https://api.lemonsqueezy.com` 换成你的域名，License Key、实例 ID、Webhook 处理逻辑无需改动；把新 Webhook 的 URL/Secret 配上。
4. 在 LS 停止服务（2027-03-31）前完成切换。

完整步骤、归档格式与回滚建议见 [docs/MIGRATION.md](docs/MIGRATION.md)。

## 使用 Stripe 登录（管理后台）

后台支持 **「使用 Stripe 登录」**（Stripe Connect OAuth）：

- 在 Stripe → Connect → OAuth 设置里启用并登记回调 `PUBLIC_URL/admin-api/auth/stripe/callback`，把 `client_id`（`ca_…`）填到 *设置 → Stripe 与登录*。
- 默认 `allowlist` 模式：只有「已绑定到某个后台用户」或在 `ADMIN_STRIPE_ACCOUNTS` 白名单里的 `acct_…` 才能登录；已登录用户可在 *个人资料* 里绑定/解绑。
- Stripe 的 OAuth 需要 `read_write` scope，所以**登录成功后会立即调用 deauthorize 撤销平台对该账号的授权**（只取身份，不留访问权），可在设置里关闭。
- 注意：Connect 平台与被登录的账号通常需要是**两个不同的 Stripe 账号**；Stripe 官方也不建议新平台使用 OAuth，它是目前唯一官方提供「Stripe 账号身份」的网页登录流程。邮箱密码登录始终可用。

## 目录结构

```
backend/    Go 服务（cmd/server, cmd/dfctl, internal/*）
frontend/   Next.js 管理后台 + 客户页面
docs/       调研、架构、API 契约、迁移指南
```

## 已知限制

- 不含：折扣码/联盟营销/用量计费（`discounts`、`affiliates`、`usage-records` 返回明确的 404）；促销码请用 Stripe 原生 Promotion Codes。
- 订阅「改套餐」请走 Stripe Billing Portal；`PATCH /v1/subscriptions` 仅支持取消/恢复/暂停。
- 订单的 `*_usd` 换算使用内置近似汇率表（可用环境变量 `FX_RATES='{"EUR":1.09}'` 覆盖，单位：1 个货币单位折合多少美元），仅用于报表，不影响实际扣款。
- 运维：每个实例需要一个公网 HTTPS 地址供 Stripe Webhook 回调。多实例部署时，Webhook 投递与导入任务用数据库租约避免重复，但限流是进程内的。
