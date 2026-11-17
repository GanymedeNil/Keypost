# 与 Lemon Squeezy API 的兼容性

基准：docs.lemonsqueezy.com（2026-11 抓取）。**把 `https://api.lemonsqueezy.com` 换成 `https://<你的域名>` 即可**；官方 SDK（`@lemonsqueezy/lemonsqueezy.js` 等）可通过自定义 base URL 使用。

## License API（无需鉴权，`application/x-www-form-urlencoded`，也接受 JSON）

| 端点 | 状态 | 说明 |
| --- | --- | --- |
| `POST /v1/licenses/activate` | ✅ | 字段、`meta`、错误文案（`This license key has reached the activation limit.` 等）一致；超限返回 400 且不含 `instance` |
| `POST /v1/licenses/validate` | ✅ | 无 `instance_id` 时 `instance: null`；key/实例不存在 404（`license_key not found.` / `license_key_instance not found.`） |
| `POST /v1/licenses/deactivate` | ✅ | 释放一个激活位，`license_key.activation_usage` 即时更新 |
| 限流 | ✅ | 每 IP 60 次/分钟（`LICENSE_RATE_LIMIT`），响应带 `X-Ratelimit-*`，超限 429 |

## JSON:API（`Authorization: Bearer <key>`，`Accept: application/vnd.api+json`）

| 资源 | 读 | 写 | 备注 |
| --- | --- | --- | --- |
| `GET /v1/users/me` | ✅ | | |
| stores | ✅ | ❌ | 创建/修改请在后台；`total_sales` 等统计实时计算 |
| customers | ✅ | ✅ create / update | `urls.customer_portal` = 24h 签名链接，访问时动态创建 Stripe Billing Portal |
| products / variants / prices | ✅ | ❌ | 与 LS 一样，目录通过后台维护；`buy_now_url` 指向本服务的结账入口 |
| files | ✅ | ❌ | `download_url` 签名 1 小时、每 IP 每天 10 次（可配）；文件对象同时含 `createdAt` 与 `created_at` |
| orders | ✅ | ✅ refund、generate-invoice | refund 调 Stripe Refund；invoice 返回 Stripe 收据/发票链接，否则本服务收据页 |
| order-items | ✅ | | |
| subscriptions | ✅ | ✅ `PATCH cancelled/pause`、`DELETE` | 取消=Stripe `cancel_at_period_end`；改套餐/用量请走 Stripe |
| subscription-invoices | ✅ | ✅ refund | |
| subscription-items | ✅ | ❌ | |
| license-keys | ✅ | ✅ `PATCH activation_limit/expires_at/disabled` | 触发 `license_key_updated` |
| license-key-instances | ✅ | | `identifier` 即 `instance_id` |
| webhooks | ✅ | ✅ create / update / delete | `secret` 6–40 位，只写 |
| checkouts | ✅ | ✅ create | 创建 Stripe Checkout Session；`url` 即 Stripe 托管页；支持 `custom_price`、`checkout_data.{email,name,custom,variant_quantities,billing_address,discount_code→allow_promotion_codes}`、`product_options.redirect_url`、`expires_at` |
| discounts / discount-redemptions / usage-records / affiliates | ❌ | ❌ | 返回 JSON:API 404；折扣请用 Stripe Promotion Codes |

通用行为：`page[number]`/`page[size]`（默认 10，最大 100）、`meta.page`、`links.first/last/next/prev`、`filter[...]`、`include=`、`sort=`、嵌套路由（`/v1/orders/1/license-keys`、`/v1/orders/1/customer` …）、错误体 `{"jsonapi":…, "errors":[{detail,status,title}]}`（校验错误带 `source.pointer`）、限流 300 次/分钟 + `X-Ratelimit-Limit/Remaining`。

## Webhook（出站）

| 项 | 状态 |
| --- | --- |
| 事件 | `order_created` `order_refunded` `customer_updated` `subscription_created/updated/cancelled/resumed/expired/paused/unpaused` `subscription_payment_success/failed/recovered/refunded` `license_key_created/updated`（`affiliate_activated` 可订阅但不会触发） |
| 头 | `Content-Type: application/json`、`X-Event-Name`、`X-Signature`（`HMAC_SHA256(secret, rawBody)` hex，直接沿用 LS 文档里的 Node/PHP/Python 验签代码） |
| Body | `{"meta":{"event_name","custom_data"},"data":{type,id,attributes,relationships,links}}` |
| 事件序 | 首单：`order_created → subscription_created → subscription_payment_success → license_key_created`；续费成功：`subscription_payment_success → subscription_updated`；失败/恢复/取消/过期序列与 LS 文档一致 |
| 重试 | 非 2xx 或超时（10s）重试，默认共 4 次（5s/25s/125s 退避） |
| 自定义数据 | `checkout_data.custom` → 订单、订阅、License、发票相关事件的 `meta.custom_data`（非本服务创建的 Stripe Checkout 则取 Session metadata） |

## 有意的差异

1. **支付方是 Stripe**：`payment_processor` 恒为 `stripe`；历史 PayPal 订阅会被导入但无法续费（LS 官方也不支持迁移 PayPal 订阅）。
2. **金额口径**：`subtotal` = Stripe `amount_subtotal`，`discount_total`/`tax` 取自 `total_details`，`total` = `amount_total`；`*_usd` 为近似换算。
3. **订单号**：本服务按店铺自增；导入的历史订单保留原订单号，新单从最大值继续。
4. **下载链接域名**：由 `app.lemonsqueezy.com` 变为你自己的域名；链接内含订单信息，退款/订阅失效后立即失效。
5. **客户门户**：`urls.customer_portal` 不再是 LS Portal，而是跳转到 Stripe Billing Portal（订阅、支付方式、发票）。
6. **ID 与 Key**：导入数据 ID 不变；新生成的 License Key 仍是 UUID v4。
