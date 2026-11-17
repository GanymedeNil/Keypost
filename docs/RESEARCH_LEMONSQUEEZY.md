# Lemon Squeezy 数字交付（Digital Fulfillment）调研

> 调研日期：2026-11-17。来源：docs.lemonsqueezy.com 的 migration / API / help 页面（原始 HTML 已抽取核对，字段以官方示例为准）。

## 1. 背景

- Lemon Squeezy 已停止接受新用户，**2027-03-31 停止处理交易**（含订阅续费），不会延期。
- 官方推荐迁移到 **Stripe Managed Payments**（Stripe 作为 MoR）。但 Managed Payments **不含**：
  文件交付、下载链接、License Key、店面、联盟营销、PayPal。
- 官方原话：数字交付「需要你自建或接入第三方」，并且**不会自动迁移**历史的 files / links / license keys。
- 迁移工具 2026-12-02 上线：只迁移 customers（给出 LS ID → Stripe ID 映射报告）、products（每个 variant → 一个 Stripe product）、
  卡类订阅。**订单历史、License Key、文件、下载权限都需要自己带走。**
- 所有 LS 链接 / API / Webhook **不会自动重定向**，集成方必须改调用地址。

=> 本项目的定位：**一个与 LS API 协议兼容的自托管交付层，数据来自 Stripe，历史数据可从 LS 导入**。
业务方只需要把 `https://api.lemonsqueezy.com` 换成自己的域名，其余代码、SDK、Webhook 处理逻辑尽量不动。

## 2. 需要兼容的能力清单

### 2.1 License API（公开，无需鉴权）

| 项 | 内容 |
| --- | --- |
| Base | `https://api.lemonsqueezy.com`，仅 HTTPS |
| 请求 | `Accept: application/json`；POST 使用 `application/x-www-form-urlencoded` |
| 限流 | 60 次/分钟 |
| 错误码 | 400（见 `error` 字段）、404（找不到）、422（缺少/非法字段） |

| 端点 | 参数 | 返回 |
| --- | --- | --- |
| `POST /v1/licenses/activate` | `license_key`*, `instance_name`* | `activated`, `error`, `license_key{id,status,key,activation_limit,activation_usage,created_at,expires_at}`, `instance{id(uuid),name,created_at}`, `meta{store_id,order_id,order_item_id,product_id,product_name,variant_id,variant_name,customer_id,customer_name,customer_email}` |
| `POST /v1/licenses/validate` | `license_key`*, `instance_id` | `valid`, `error`, `license_key`, `instance`（无 instance_id 时为 `null`）, `meta` |
| `POST /v1/licenses/deactivate` | `license_key`*, `instance_id`* | `deactivated`, `error`, `license_key`, `meta` |

License key 状态：`inactive`（无激活）/ `active`（≥1 激活）/ `expired` / `disabled`。
激活达到上限时返回 `activated:false`，`error:"This license key has reached the activation limit."`，且**不含** `instance`。

### 2.2 JSON:API（Bearer 鉴权）

- 头：`Accept: application/vnd.api+json`、`Content-Type: application/vnd.api+json`、`Authorization: Bearer <key>`。
- 分页：`page[number]`、`page[size]`（默认 10，最大 100）；响应含 `meta.page{currentPage,from,lastPage,perPage,to,total}` 与 `links{first,last,next,prev}`。
- 过滤：`filter[xxx]=`；关联：`include=`；嵌套：`/v1/products/100/variants`；4xx 返回 `{"jsonapi":{"version":"1.0"},"errors":[{"detail","status","title"}]}`。
- 限流：300 次/分钟，响应头 `X-Ratelimit-Limit` / `X-Ratelimit-Remaining`，超限 429。
- Test mode：测试模式 Key 只能看到测试数据，对象带 `test_mode`。

交付相关资源：

| 资源 | 端点 | 说明 |
| --- | --- | --- |
| users | `GET /v1/users/me` | 当前 Key 对应用户 |
| stores | `GET /v1/stores`, `/v1/stores/{id}` | |
| customers | `POST/GET/PATCH /v1/customers`，`GET /v1/customers/{id}` | `urls.customer_portal` 预签名 24h |
| products | `GET /v1/products`, `/{id}` | `buy_now_url` |
| variants | `GET /v1/variants`, `/{id}` | `has_license_keys`, `license_activation_limit`, `license_length_*`, `links[]` |
| prices | `GET /v1/prices`, `/{id}` | 价格模型、订阅周期、试用 |
| files | `GET /v1/files`, `/{id}` | `download_url` 签名、**1 小时过期、每 IP 每天 10 次** |
| orders | `GET /v1/orders`, `/{id}`；`POST /v1/orders/{id}/generate-invoice`；`POST /v1/orders/{id}/refund` | `urls.receipt` |
| order-items | `GET /v1/order-items`, `/{id}` | |
| subscriptions | `GET/PATCH/DELETE /v1/subscriptions` | `DELETE` = cancel |
| subscription-invoices | `GET /v1/subscription-invoices`, `/{id}` | |
| subscription-items | `GET/PATCH /v1/subscription-items` | |
| license-keys | `GET /v1/license-keys`, `/{id}`；`PATCH /v1/license-keys/{id}` | 可改 `activation_limit` / `expires_at` / `disabled` |
| license-key-instances | `GET /v1/license-key-instances`, `/{id}` | `identifier` = `instance_id` |
| webhooks | `POST/GET/PATCH/DELETE /v1/webhooks` | `secret` 只写不读 |
| checkouts | `POST /v1/checkouts`, `GET /v1/checkouts/{id}` | 本项目映射为 Stripe Checkout Session |

不在范围：discounts / discount-redemptions / affiliates / usage-records（折扣与联盟交由 Stripe 与独立平台）。
导入的历史订单仍保留 `discount_total`、`affiliate_id` 等字段值。

### 2.3 Webhook（出站）

- `POST` 到商家配置的 URL，`Content-Type: application/json`，头：`X-Event-Name`、`X-Signature`。
- 签名：`HMAC-SHA256(secret, rawBody)` 的 **hex digest**（常量时间比较）。secret 长度 6–40。
- Body：`{"meta":{"event_name","custom_data"?}, "data":{type,id,attributes,relationships,links}}`。
- 事件：`order_created`, `order_refunded`, `customer_updated`, `subscription_created|updated|cancelled|resumed|expired|paused|unpaused`,
  `subscription_payment_success|failed|recovered|refunded`, `license_key_created`, `license_key_updated`, `affiliate_activated`（不实现）。
- 事件序（首单）：`order_created` → `subscription_created` → `subscription_payment_success` → `license_key_created`。
- 续费成功：`subscription_payment_success` → `subscription_updated`；失败：`subscription_payment_failed` → `subscription_updated`；
  恢复：`subscription_payment_success` + `subscription_payment_recovered` → `subscription_updated`。
- Checkout 可传 `custom_data`，随 Order / Subscription / License key 相关事件的 `meta.custom_data` 回传。

### 2.4 客户侧（My Orders / 收据）

- 客户用下单邮箱获取 **magic link** 登录 My Orders，可查看订单、订阅、**当前可用文件**、链接、License Key。
- 商家新增 / 替换文件后，**所有历史客户立即可见**；删除后立即不可见。
- 收据邮件含 License Key 与下载入口；链接变量：`[license_key] [order_id] [order_identifier] [email] [name] [total]`。
- 订阅过期后 License Key 随之过期；订阅 License 的有效期绑定订阅生命周期。
- 订单导出 CSV 列：`identifier, order_number, product_name, variant_name, user_name, user_email, city, country, postal_code,
  card_brand, card_last_four, currency, currency_rate, subtotal, discount_total, tax, total, tax_name, tax_rate, tax_number, date_utc`。

### 2.5 License Key 语义

- Variant 上：`has_license_keys`、`license_activation_limit`（或 unlimited）、`license_length_value/unit(days|months|years)`（或 unlimited）。
- 订阅 variant：key 过期 = 订阅过期（取消后到期、或 dunning 结束）。
- 全额退款：License Key 置为 disabled。
- Key 格式：UUID v4（例：`38b1460a-5104-4067-a91d-77b872934d51`）；`key_short` = `XXXX-` + 末 12 位。

## 3. Stripe 侧约定

- 履约事件：`checkout.session.completed`（`payment_status=paid|no_payment_required`），延迟支付等 `checkout.session.async_payment_succeeded`；
  需用 API 拉取 `line_items`（webhook 里不含），履约必须幂等。
- 订阅：`invoice.paid`、`invoice.payment_failed`、`customer.subscription.updated|deleted`。退款：`charge.refunded`。客户：`customer.updated`。
- API 版本差异（basil 起）：`current_period_end` 从 Subscription 移到 `items.data[].current_period_end`；
  Invoice 的 `subscription` 移到 `parent.subscription_details.subscription`。**解析必须同时兼容两种形状**。
- Managed Payments：创建 Checkout Session 时传 `managed_payments[enabled]=true`（需要 Preview API 版本，且与 Connect 不兼容）。
- Stripe 登录：Connect OAuth（`connect.stripe.com/oauth/authorize` → `/oauth/token` 返回 `stripe_user_id`）。
  平台需 `read_write` scope；官方不推荐新平台使用 OAuth，但这是唯一官方的「账号身份」登录流程。本项目登录后立即 `deauthorize`，只保留身份。

## 4. 与本项目的映射

| LS 概念 | 本项目 |
| --- | --- |
| Store | `stores`（支持多店；登录账号可绑定） |
| Product / Variant / Price | 保留 LS ID；`variants.stripe_product_id` / `prices.stripe_price_id` 对应 Stripe |
| File | 本地磁盘 / S3 兼容存储；签名下载 URL（1h，10 次/天/IP） |
| Customer | 保留 LS ID；`customers.stripe_customer_id` 由迁移映射表（LS ID → Stripe ID）导入 |
| Order / OrderItem | 来自 Stripe `checkout.session.completed`；历史订单由 LS 导入 |
| Subscription | 来自 Stripe；历史订阅导入后按 `stripe_subscription_id` 映射对接 |
| License key / instance | **原样导入**（key 字符串与 instance UUID 不变），老客户端无需改 key |
| Webhook | 同名事件、同签名算法、同 payload 形状 |
| Customer Portal | `/billing` 预签名 → 动态创建 Stripe Billing Portal Session |
| Checkout | `POST /v1/checkouts` → Stripe Checkout Session；`buy_now_url` → `/checkout/buy/{variant_slug}` |
