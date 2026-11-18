# 架构说明

## 1. 组件

| 组件 | 路径 | 职责 |
| --- | --- | --- |
| API 服务 | `backend/cmd/server` | LS 兼容 API、License API、Stripe Webhook、客户 API、Admin API、下载、后台任务 |
| 运维 CLI | `backend/cmd/dfctl` | `migrate`、`admin create / reset-password / reset-mfa`、`apikey create`（直接操作数据库） |
| Web | `frontend/` | Next.js：`/admin/*` 管理后台、`/my-orders/*` 客户页面；`rewrites` 反代到后端 |

后端包划分（`backend/internal`）：

```
config      环境变量               crypt      AES-GCM 加密、HMAC 签名、随机令牌
db          GORM 模型/迁移/连接     settings   运行期可改配置（DB 加密存储，env 兜底）
urls        带签名的公开 URL        resource   DB 模型 → LS JSON:API 对象（REST 与 Webhook 共用）
lsapi       /v1 JSON:API + License API     admin      /admin-api（后台）
storefront  下载/结账跳转/账单门户/客户 API/Stripe Webhook 入口
service     业务核心：履约、订阅、退款、License、结账、Stripe 事件分发
stripe      轻量 Stripe REST 客户端 + Webhook 验签 + OAuth
webhooks    出站 Webhook 投递（队列、签名、重试、重放）
importer    LS 数据导入、Stripe 映射/自动匹配
storage     本地磁盘 / S3 兼容   mail   SMTP 与邮件模板   ratelimit / httpx / auth
```

## 2. 数据流

### 购买（一次性）

1. 买家通过 `buy_now_url` 或 `POST /v1/checkouts` 创建 Stripe Checkout Session（`client_reference_id` = 本地 checkout id，自定义数据存本地，避免 Stripe metadata 500 字符限制）。
2. Stripe 回调 `/webhooks/stripe`：验签（5 分钟容差，支持多个密钥便于轮换）→ 写入 `stripe_events`（事件 ID 主键，天然去重）→ 处理。
3. `checkout.session.completed` → **重新拉取** Session（含 `line_items.data.price.product`、`payment_intent.latest_charge`…）→ 在**一个事务**里：upsert 客户、分配订单号、写订单/明细、（订阅）建订阅、按变体规则签发 License Key。
4. 事务提交后按固定顺序发出站事件：`order_created` → `subscription_created` → `subscription_payment_success` → `license_key_created`，再异步发收据邮件。
5. 买家被重定向到 `/checkout/success`，等待履约（必要时同步履约一次）后跳转到带签名的订单页或你的 `redirect_url`（支持 `[license_key]` 等变量）。

**幂等**：`stripe_events.id` 主键 + `orders.stripe_checkout_session` 唯一索引 + 先查后写；重复投递、并发投递、`async_payment_succeeded` 与 `completed` 同时到达都只会生成一个订单。处理失败返回 5xx 让 Stripe 重试，同时有后台任务重跑 `failed` 事件。

### 订阅生命周期

对每个 `customer.subscription.*` / `invoice.*` 事件，**不信任事件载荷**，而是按 ID 重新拉取最新对象再对账（避免乱序、API 版本差异）。`mapSubscription` 把 Stripe 状态映射为 LS 状态（`trialing→on_trial`、`cancel_at_period_end→cancelled`（宽限期）、`canceled→expired`、`pause_collection→paused` …），仅在状态快照变化时发 `subscription_cancelled/resumed/expired/paused/unpaused` + `subscription_updated`。订阅绑定的 License Key：取消后到 `ends_at` 过期、恢复后复活、过期后立即失效。

### 退款

`charge.refunded` 与后台/API 发起的退款都收敛到 `applyOrderRefund`，**按累计金额设置**（不是累加），所以重复事件无副作用，也不会重复发 `order_refunded`。全额退款默认禁用该订单的 License Key（可关）。

### 出站 Webhook

事件 → 为每个匹配的 endpoint 生成 `webhook_deliveries`（payload 已渲染并固化，重放不受后续数据变化影响）→ 后台 worker 租约领取：**同一 endpoint 的待投递事件按创建顺序串行**（首单的 `order_created` 先于 `license_key_created`），不同 endpoint 并发；与 LS 一样不保证全局严格有序，消费端应以 `subscription_updated` 等事件的最新数据为准；失败按 5s/25s/125s 退避重试，用尽后置为 `failed`，可在后台重投。目标地址拒绝链路本地地址（云元数据）。

## 3. 兼容 LS 的关键设计

- **ID**：所有主键为 int64；导入保留 LS 原 ID（含 `license_keys.id`、实例 `identifier`、订单 `identifier/order_number`），新数据自增继续（PostgreSQL 导入后重置序列）。
- **序列化**：`resource` 包按 LS 文档字段顺序生成对象（含 `relationships` 链接、`first_order_item`、`urls.receipt/customer_portal`、`status_formatted` 等）。`files` 保留 LS 文档里的 `createdAt/updatedAt` 驼峰字段，同时提供下划线字段。
- **License 状态**：由 `disabled / expires_at / instances_count` 实时推导 `inactive|active|expired|disabled`；激活在事务内加行锁（PostgreSQL `FOR UPDATE`；SQLite 立即事务）检查上限，避免并发超发。
- **测试模式**：API Key 带 `test_mode`，所有资源查询按它过滤；Stripe 测试密钥（`sk_test_`）产生的数据 `test_mode=true`。
- **签名 URL**：收据 `…/my-orders/{uuid}?signature=`（永久有效的只读链接）、下载 `…/download/{uuid}?expires=&order=&signature=`（默认 1 小时，且按订单校验退款/订阅状态）、账单门户 `/billing?…`（24 小时，访问时再动态创建 Stripe Billing Portal 会话）。

## 4. 安全模型

| 面 | 机制 |
| --- | --- |
| 密钥落盘 | Stripe 密钥、Webhook 签名密钥、SMTP 密码、出站 Webhook secret 用 `APP_SECRET` 派生密钥做 AES-256-GCM 加密；API 永不回显明文（只给 `set/source/hint`） |
| 后台会话 | 随机令牌，库中仅存 SHA-256；HttpOnly + SameSite=Lax Cookie；所有写请求要求 `X-Requested-With: df-admin`；登录按 IP 与账号限流；密码 bcrypt(12) 并对不存在账号做等价耗时比较 |
| 两步验证 | 所有成员（含「使用 Stripe 登录」的）登录后会话都是未验证状态，只能访问 `/me`、`/auth/mfa/*` 和退出；通过 TOTP（RFC 6238）或一次性恢复码后才可用。密钥用 `APP_SECRET` 加密存库、恢复码存带密钥的哈希；同一时间窗的验证码不能重放；连续错 5 次锁定账号 15 分钟；owner 可重置他人，最后手段是 `dfctl admin reset-mfa`。`MFA_REQUIRED=false` 仅用于本地开发 |
| API Key | `dfk_live_…/dfk_test_…`，库中仅存 SHA-256，创建时只显示一次，可设过期/吊销 |
| 角色 | `owner` > `admin` > `viewer`（所有写操作 403）；不能删除/降级最后一个 owner |
| 客户登录 | 魔法链接 15 分钟、一次性、库中存哈希；对不存在的邮箱同样返回 204（防枚举）；按 IP/邮箱限流；会话 Cookie 为 HMAC 签名 |
| 下载 | 签名+过期；每文件每 IP 每日次数限制；退款/订阅失效后拒绝；本地存储做路径逃逸保护 |
| Stripe Webhook | 常量时间验签、时间戳容差、事件级去重 |
| Stripe 登录 | `state` 绑定到一次性签名 Cookie；登录后立刻 deauthorize；默认仅白名单/已绑定账号 |

## 5. 需要订阅的 Stripe 事件

`checkout.session.completed`、`checkout.session.async_payment_succeeded`、`invoice.paid`、`invoice.payment_failed`、
`customer.subscription.updated|deleted|paused|resumed`、`charge.refunded`、`customer.updated`。
（后台「创建 / 更新 Webhook Endpoint」会自动配置。）

## 6. 数据模型速览

`stores → products → variants → prices / files`；`customers`；`orders → order_items`；`subscriptions → subscription_items / subscription_invoices`；
`license_keys → license_key_instances`；`checkouts`；`webhooks → webhook_deliveries`；`stripe_events`；`users / admin_sessions / api_keys / audit_logs / settings`；
`customer_login_tokens / email_logs / download_logs`；`import_jobs / import_logs`。
迁移用 GORM `AutoMigrate`（启动时自动执行，只增不删）。
