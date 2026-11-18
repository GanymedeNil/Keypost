# Admin API 契约（`/admin-api/*`）与客户 API（`/storefront-api/*`）

前端（Next.js）与后端（Go）以本文件为准。**后端按此实现，前端按此消费。** 改动需两边同步。

## 0. 通用约定

- Base：同源，路径前缀 `/admin-api`。Next.js `rewrites` 把 `/admin-api/*`、`/storefront-api/*`、`/v1/*`、`/download/*`、`/billing`、`/checkout/*`、`/webhooks/*`、`/healthz` 反代到 Go（`BACKEND_URL`，默认 `http://localhost:8080`）。
- 鉴权：登录后服务端下发 HttpOnly Cookie `df_admin`。**所有 POST/PUT/PATCH/DELETE 必须带请求头 `X-Requested-With: df-admin`**（CSRF 防护），否则 403。
- 未登录 → `401 {"error":{"code":"unauthenticated",...}}`；权限不足 → `403 {"error":{"code":"forbidden"}}`。
- 角色：`owner`（全部）> `admin`（除「用户管理 / API Key 创建与吊销 / 设置写入」外全部）> `viewer`（只读，写操作 403；**例外**：`PATCH /me`、`POST /me/password`、`DELETE /me/stripe` 任何角色都可调用）。详见 §8。
- 请求/响应均为 `application/json`（上传除外：`multipart/form-data`）。字段 snake_case。时间 RFC3339 UTC（`2026-11-17T09:30:00Z`）；金额为整数（最小货币单位，美分）。
- 错误：`{"error":{"code":"validation_failed","message":"…","fields":{"email":"is invalid"}}}`；常见 code：`unauthenticated forbidden not_found validation_failed conflict stripe_error rate_limited internal`。
- 列表：查询参数 `page`（默认 1）、`per_page`（默认 25，最大 100）、`q`（搜索）、`sort`（如 `-created_at`，默认各资源不同）、`test_mode=true|false`（默认 `false`）、`store_id`、各资源自有过滤。响应：

```json
{"data":[ ... ],"meta":{"page":1,"per_page":25,"total":132,"last_page":6}}
```

- 单个资源：直接返回对象（不再包 `data`）。删除成功：`204`。
- `ID` 一律为数字（JSON number）。

## 1. 认证

| 方法 路径 | 说明 |
| --- | --- |
| `GET /auth/config` | 无需登录。`{"password_login":true,"stripe_login":true,"setup_required":false}` |
| `POST /auth/setup` | 无用户时创建 owner：`{"setup_token","email","name","password"}`（`setup_token` 在服务首次启动日志中输出，或使用 `ADMIN_EMAIL/ADMIN_PASSWORD` 环境变量引导）。成功后自动登录 |
| `POST /auth/login` | `{"email","password"}` → `{"user":User}` 并 Set-Cookie |
| `POST /auth/logout` | 204 |
| `GET /auth/stripe/start?mode=login` | 浏览器整页跳转（302）到 Stripe 授权页；`mode=link`（需已登录）把 Stripe 账号绑定到当前用户 |
| `GET /auth/stripe/callback` | Stripe 回调（后端处理）。成功 302 → `/admin`（link 模式 → `/admin/settings/profile?linked=1`）；失败 302 → `/admin/login?error=<code>`。error 取值：`stripe_denied stripe_state stripe_not_allowed stripe_exchange disabled` |
| `GET /me` | `User` |
| `PATCH /me` | `{"name"}` |
| `POST /me/password` | `{"current_password","new_password"}`（≥10 位） |
| `DELETE /me/stripe` | 解除 Stripe 账号绑定（须已设密码，否则 409） |

`User`：`{id,email,name,role,stripe_account_id,stripe_account_label,disabled,has_password,mfa_enabled,last_login_at,created_at}`；`/me`、`/auth/login`、`/auth/setup`、`/auth/mfa/verify` 里另有 `mfa`（见 §1.1）。

登录页应展示两种方式：邮箱密码表单、「使用 Stripe 登录」按钮（`stripe_login=true` 时，链接到 `/admin-api/auth/stripe/start?mode=login`）。

### 1.1 强制两步验证（MFA，TOTP）

`MFA_REQUIRED`（环境变量，默认 `true`）为真时，**所有成员**——不论用邮箱密码还是「使用 Stripe 登录」——每次登录后会话都是**未验证**状态，必须再通过一次 TOTP（或恢复码）才能使用后台。没有开关可让成员自己关闭 MFA。

- **未验证会话只能调用**：`GET /me`、`POST /auth/logout`、`/auth/mfa/*`。其余 `/admin-api/*` 一律 `403`：`{"error":{"code":"mfa_setup_required"|"mfa_required","message":"…"}}`（已绑定 → `mfa_required`，未绑定 → `mfa_setup_required`）。前端据此跳转 `/admin/mfa`。
- `/v1/*` 的 API Key 是机器凭据，不受影响。
- `GET /me` 与 `POST /auth/login`（以及 `/auth/setup`）返回的 `User` 多一个 `mfa`：`{"required":true,"enabled":false,"verified":false,"recovery_codes_left":0}`；`enabled` = 已绑定验证器，`verified` = **当前会话**已通过验证。`users` 列表项只带 `mfa_enabled`。
- 算法：RFC 6238，SHA-1、6 位、30 秒，接受前后各一个时间窗；同一时间窗的验证码不能重复使用。密钥用 `APP_SECRET` 加密存库，恢复码只存哈希。

| 方法 路径 | 说明 |
| --- | --- |
| `POST /auth/mfa/setup` | 开始绑定：生成新密钥（覆盖上一次未完成的）。已绑定 → `409`。返回 `{"secret":"BASE32…","otpauth_uri":"otpauth://totp/…"}`，前端把 `otpauth_uri` 渲染成二维码，同时显示 `secret` 供手动输入 |
| `POST /auth/mfa/enable` | `{"code":"123456"}` 校验通过后正式启用，会话变为已验证，返回 `{"recovery_codes":["xxxxx-xxxxx",…10 个]}`——**仅此一次**显示。错误 → `422`（`fields.code`） |
| `POST /auth/mfa/verify` | `{"code":"123456"}` 或 `{"recovery_code":"xxxxx-xxxxx"}`；通过后会话变为已验证，返回 `User`。连续错误 5 次锁定该账号 15 分钟（`429 rate_limited`，`message` 带剩余分钟）；恢复码一次性 |
| `POST /me/mfa/recovery-codes` | 已验证会话 + `{"code":"123456"}` → 作废旧恢复码并返回新的 10 个 `{"recovery_codes":[…]}` |
| `POST /users/{id}/mfa/reset` | **owner**：清除该成员的验证器和恢复码并让其所有会话失效，下次登录重新绑定（用于换手机/丢设备）。不能对自己使用（`409`），审计 `user.mfa_reset` |

最后手段（例如唯一的 owner 丢了设备）：服务器上执行 `dfctl admin reset-mfa --email you@example.com`。审计动作：`auth.mfa_enabled`、`auth.mfa_verified`、`auth.mfa_failed`、`auth.mfa_recovery_used`、`auth.mfa_recovery_regenerated`、`user.mfa_reset`。

## 2. 仪表盘

`GET /dashboard?store_id=&test_mode=&days=30`

```json
{
  "currency":"USD",
  "totals":{"revenue":1234500,"orders":321,"refunded":12000,"customers":210,"active_subscriptions":55,"mrr":99000,"license_keys":300,"active_license_keys":120,"downloads":4021},
  "previous":{"revenue":1000000,"orders":280},
  "series":[{"date":"2026-11-01","revenue":10000,"orders":3,"refunds":0}],
  "recent_orders":[Order],
  "recent_events":[{"kind":"stripe_event|webhook_delivery|import","title":"checkout.session.completed","status":"processed","at":"…"}],
  "health":{"stripe_configured":true,"stripe_webhook_secret_set":true,"smtp_configured":true,"failed_stripe_events":0,"failed_webhook_deliveries":2,"pending_webhook_deliveries":0,"storage_driver":"local"}
}
```

## 3. 资源

下面 `[]` 表示可选；`R`=只读列表+详情，`CRUD` 表示 `GET list / POST / GET one / PATCH / DELETE`。

### stores — 读：任何已登录角色；写：owner/admin
`Store`：`{id,name,slug,domain,url,avatar_url,plan,country,currency,support_email,email_from_name,receipt_subject,receipt_intro,stripe_account_id,total_sales,total_revenue,thirty_day_sales,thirty_day_revenue,created_at,updated_at}`
POST/PATCH 字段：`name,slug,domain,url,avatar_url,country,currency,support_email,email_from_name,receipt_subject,receipt_intro`。

### products — CRUD
列表过滤：`status`、`q`。`Product`：`{id,store_id,name,slug,description,status(draft|published),thumb_url,large_thumb_url,stripe_product_id,test_mode,buy_now_url,variants_count,from_price,to_price,created_at,updated_at}`
详情追加 `variants:[Variant]`。POST/PATCH：`store_id,name,slug,description,status,thumb_url,large_thumb_url,stripe_product_id,test_mode`。DELETE 级联删除 variants/prices/files（先要求无订单引用，否则 409）。

### variants — CRUD
过滤：`product_id`。`Variant`：
`{id,product_id,product_name,name,slug,description,links:[{title,url}],sort,status(pending|draft|published),has_license_keys,license_activation_limit,is_license_limit_unlimited,license_length_value,license_length_unit(days|months|years),is_license_length_unlimited,stripe_product_id,buy_now_url,prices:[Price],files_count,test_mode,created_at,updated_at}`
详情追加 `files:[File]`。POST 时可同时传 `price:{…Price 字段}` 一并创建。

### prices — CRUD
过滤：`variant_id`。`Price`：`{id,variant_id,category(one_time|subscription|lead_magnet|pwyw),scheme(standard|package|graduated|volume),unit_price,unit_price_decimal,setup_fee_enabled,setup_fee,package_size,tiers:[{last_unit,unit_price,unit_price_decimal,fixed_fee}],renewal_interval_unit(day|week|month|year),renewal_interval_quantity,trial_interval_unit,trial_interval_quantity,min_price,suggested_price,tax_code,currency,stripe_price_id,created_at,updated_at}`

### files
- `GET /files?variant_id=&status=` 列表；`GET /files/{id}`；`PATCH /files/{id}`（`name,version,sort,status`）；`DELETE /files/{id}`。
- `POST /variants/{id}/files`：`multipart/form-data`，字段 `file`（必填）、`name`、`version`、`status`（默认 published）。返回 `File`。上传体积上限由后端 `MAX_UPLOAD_MB`（默认 1024）限制。
- `GET /files/{id}/download`：管理员直接下载（Cookie 鉴权）。
- `File`：`{id,variant_id,variant_name,product_name,identifier,name,extension,size,size_formatted,version,sort,status(draft|published),storage_driver,content_type,sha256,downloads_count,test_mode,created_at,updated_at}`

### customers — R + PATCH
过滤：`q`（姓名/邮箱）、`country`。`Customer`：`{id,store_id,name,email,status,city,region,country,stripe_customer_id,total_revenue,mrr,orders_count,subscriptions_count,license_keys_count,test_mode,created_at,updated_at}`
详情追加 `orders:[Order]`、`subscriptions:[Subscription]`、`license_keys:[LicenseKey]`。
`PATCH`：`name,email,city,region,country,stripe_customer_id`。
`POST /customers/{id}/portal-link` → `{"url":"https://billing.stripe.com/…"}`（Stripe Billing Portal 会话）。
`POST /customers` 手动创建：`store_id,name,email`。

### orders — R + 动作
过滤：`status`、`customer_id`、`q`（订单号/邮箱/姓名）、`created_from`、`created_to`（RFC3339）。默认 `-created_at`。
`Order`：`{id,store_id,customer_id,identifier,order_number,user_name,user_email,currency,currency_rate,subtotal,setup_fee,discount_total,tax,total,refunded_amount,subtotal_usd,discount_total_usd,tax_usd,total_usd,refunded_amount_usd,tax_name,tax_rate,tax_inclusive,status(pending|failed|paid|refunded|partial_refund|fraudulent),refunded,refunded_at,affiliate_id,referral_amount,country,city,postal_code,card_brand,card_last_four,custom_data,stripe_checkout_session,stripe_payment_intent,stripe_invoice_id,stripe_customer_id,source(stripe|lemonsqueezy|manual),test_mode,receipt_url,first_item:{product_name,variant_name,price,quantity},created_at,updated_at}`
详情追加 `items:[OrderItem]`、`license_keys:[LicenseKey]`、`subscriptions:[Subscription]`、`files:[File]`（该订单当前可下载文件）、`emails:[EmailLog]`、`webhook_deliveries:[Delivery]`。
`OrderItem`：`{id,order_id,product_id,variant_id,price_id,product_name,variant_name,price,quantity}`
- `POST /orders/{id}/refund`：`{"amount":1000?,"reason":"requested_by_customer"?}`；省略 amount = 全额。调用 Stripe Refund，成功后本地状态由 webhook 更新（同时本地乐观更新）。
- `POST /orders/{id}/resend-receipt`：`{"email"?}` → 204。
- `GET /orders/export.csv?…同列表过滤`：CSV 下载（列同 LS 导出）。

### subscriptions — R + 动作
过滤：`status`、`customer_id`、`product_id`、`q`。`Subscription`：`{id,store_id,customer_id,order_id,order_item_id,product_id,variant_id,price_id,product_name,variant_name,user_name,user_email,status(on_trial|active|paused|past_due|unpaid|cancelled|expired),status_formatted,card_brand,card_last_four,payment_processor,pause,cancelled,trial_ends_at,billing_anchor,quantity,renews_at,ends_at,stripe_subscription_id,stripe_customer_id,test_mode,created_at,updated_at}`
详情追加 `invoices:[SubscriptionInvoice]`、`license_keys:[LicenseKey]`。
- `POST /subscriptions/{id}/cancel`（Stripe：`cancel_at_period_end=true`）；`POST /subscriptions/{id}/resume`；`POST /subscriptions/{id}/sync`（从 Stripe 重新拉取并对账）；`POST /subscriptions/{id}/link-stripe`：`{"stripe_subscription_id"}`（迁移后手工对接）。
- `SubscriptionInvoice`：`{id,subscription_id,customer_id,user_name,user_email,billing_reason,card_brand,card_last_four,currency,status(pending|paid|void|refunded|partial_refund),refunded,refunded_at,subtotal,discount_total,tax,total,refunded_amount,invoice_url,stripe_invoice_id,created_at}`；`GET /subscription-invoices?subscription_id=`。

### license-keys
过滤：`status`（inactive|active|expired|disabled）、`order_id`、`customer_id`、`product_id`、`variant_id`、`q`（key / 邮箱）。
`LicenseKey`：`{id,store_id,customer_id,order_id,order_item_id,product_id,variant_id,subscription_id,product_name,variant_name,user_name,user_email,key,key_short,activation_limit(null=不限),instances_count,disabled,status,status_formatted,expires_at,test_mode,created_at,updated_at}`；详情追加 `instances:[Instance]`。
- `PATCH /license-keys/{id}`：`{"activation_limit":5|null,"expires_at":"…"|null,"disabled":true|false}`
- `POST /license-keys`：手动签发 `{"order_id"?, "customer_id"?, "variant_id", "user_email"?, "user_name"?, "activation_limit"?, "expires_at"?}`（无订单时必须给 `customer_id` 或 `user_email`）。
- `POST /license-keys/{id}/regenerate`：重新生成 key 字符串并清空激活实例。
- `GET /license-key-instances?license_key_id=`；`DELETE /license-key-instances/{id}`（释放一个激活位）。
- `Instance`：`{id,license_key_id,identifier,name,created_at,updated_at}`

### webhooks（出站，给商家业务系统的）— CRUD
`Webhook`：`{id,store_id,url,events:[string],disabled,secret_set,last_sent_at,test_mode,stats:{success_24h,failed_24h},created_at,updated_at}`；POST/PATCH 字段：`store_id,url,events,secret(6–40 位，只写),disabled,test_mode`。
- `GET /webhook-events` → `["order_created",…]`（可选事件枚举）。
- `POST /webhooks/{id}/test`：发一个 `order_created` 样例，返回对应 `Delivery`（同步等待）。
- `GET /webhook-deliveries?webhook_id=&status=&event_name=`；`GET /webhook-deliveries/{id}`（追加 `payload` 字符串）；`POST /webhook-deliveries/{id}/redeliver`。
- `Delivery`：`{id,webhook_id,webhook_url,event_name,resource_type,resource_id,status(pending|success|failed),attempts,next_attempt_at,last_status_code,last_response,last_error,duration_ms,created_at,delivered_at}`

### stripe-events — R
`GET /stripe-events?status=&type=&q=`；`GET /stripe-events/{id}`（含 `payload`）；`POST /stripe-events/{id}/replay`（重新处理）。
`StripeEvent`：`{id,type,account,livemode,status(received|processed|ignored|failed),attempts,error,received_at,processed_at}`

### api-keys（LS 兼容 `/v1/*` 使用的 Bearer Key）— 列表 owner/admin，创建/吊销 owner
`GET /api-keys`；`POST /api-keys` `{"name","test_mode","expires_at"?}` → `{...APIKey,"token":"dfk_live_…"}`（**token 仅此一次返回**）；`DELETE /api-keys/{id}`（吊销）。
`APIKey`：`{id,name,prefix,test_mode,owner_email,last_used_at,expires_at,revoked_at,created_at}`

### users（后台成员）— owner
CRUD：`POST {"email","name","role","password"?,"stripe_account_id"?}`；`PATCH {name,role,disabled,password?,stripe_account_id?}`；不可删除/降级最后一个 owner。

### audit-logs — R
`GET /audit-logs?action=&user_email=&resource_type=` → `{id,user_email,action,resource_type,resource_id,details,ip,created_at}`

### email-logs — R
`GET /email-logs?status=&q=` → `{id,order_id,to,subject,template,status,error,created_at}`；`POST /email-logs/test` `{"to"}` 发测试邮件。

## 4. Stripe 集成

- `GET /stripe/status` → `{configured,livemode,account:{id,business_name,country,email},webhook_secret_set,webhook_url,required_events:[…],error?}`
- `POST /stripe/webhook-endpoint`：调用 Stripe API 在 `webhook_url` 上创建/更新 webhook endpoint（必需事件），把签名密钥保存到设置，返回 `{id,url,events,created}`。
- `GET /stripe/catalog` → Stripe Products + Prices：`[{id,name,active,description,metadata,default_price,prices:[{id,unit_amount,currency,recurring:{interval,interval_count}|null,active}],mapped_variant_id|null}]`
- `POST /stripe/catalog/import`：`{"store_id","stripe_product_ids":[…]?,"publish":true}` 把 Stripe 目录建成 Product/Variant/Price（含 Stripe 映射）。
- `POST /variants/{id}/link-stripe`：`{"stripe_product_id","stripe_price_id"}`；`DELETE /variants/{id}/link-stripe`。

## 5. 设置

`GET /settings`（owner/admin；密钥字段永不回显明文）

```json
{
  "urls":{"public_url":"https://pay.example.com","stripe_webhook_url":"https://pay.example.com/webhooks/stripe","stripe_oauth_redirect_uri":"https://pay.example.com/admin-api/auth/stripe/callback","api_base_url":"https://pay.example.com/v1"},
  "stripe":{"secret_key":{"set":true,"source":"db","hint":"sk_live_…1234"},"webhook_secret":{"set":true,"source":"env","hint":"whsec_…ab12"},"api_version":"2025-09-30.clover","managed_payments":false,
            "oauth_client_id":"ca_…","oauth_secret_key":{"set":false},"login_mode":"allowlist","login_accounts":["acct_123"],"login_auto_role":"viewer","oauth_revoke_after_login":true},
  "smtp":{"host":"","port":587,"user":"","password":{"set":false},"from":"","security":"starttls"},
  "license":{"disable_on_refund":true},
  "download":{"limit_per_day":10,"url_ttl_seconds":3600},
  "checkout":{"success_url":"","cancel_url":""}
}
```

`PUT /settings`（owner）：同结构的**部分**更新。密钥字段传字符串=设置，不传=保持，`""`=清除。返回与 GET 相同。
`POST /settings/smtp/test` `{"to"}`。

## 6. 数据迁移（imports）

- `GET /imports`；`GET /imports/{id}`；`GET /imports/{id}/logs?after=<logId>`（增量日志，`[{id,level,message,created_at}]`）；`POST /imports/{id}/cancel`。
- `ImportJob`：`{id,kind,status(pending|running|completed|failed|cancelled),options,progress:{stage,done,total,counts:{products:3,…},warnings:2},error,created_by,started_at,finished_at,created_at}`
- `POST /imports/lemonsqueezy`（从 LS API 拉取）：`{"api_key","test_mode":false,"store_ids":[…]?,"resources":["stores","products","variants","prices","files","customers","orders","subscriptions","license_keys"]?,"download_files":true,"dry_run":false,"on_conflict":"abort"}` → `ImportJob`（异步）。`on_conflict`：`"abort"`（默认）遇到"ID 已被本库另一条不同数据占用"就停止并以 `failed` 结束；`"overwrite"` 覆盖本地数据（逐条警告）。其他取值 → 422 `/on_conflict`。试运行同样检查，列出全部冲突后以 `failed` 结束。详见 `docs/MIGRATION.md` §0。
- `POST /imports/lemonsqueezy/archive`（离线导入）：`multipart`，字段 `file`（ZIP，内含各资源的 JSON:API 分页导出，见 `docs/MIGRATION.md`）、`download_files`、`dry_run`、`test_mode`、`on_conflict`（同上）。
- `POST /imports/stripe-mapping`：`multipart`，字段 `file`（LS 迁移工具给出的「LS Customer ID → Stripe Customer ID」CSV）、可选 `kind=customers|products|subscriptions`（默认 customers）。
- `POST /imports/stripe-auto-match`：`{"customers":true,"products":true,"subscriptions":true}` 通过 Stripe API 以邮箱 / 产品名自动匹配，未匹配项写入日志。
- `GET /imports/mapping-report.csv`：当前未建立 Stripe 映射的 customers / variants / subscriptions 清单（CSV 下载）。

## 7. 客户端（公开）API `/storefront-api/*`（无需 admin 登录）

Cookie `df_customer`（HttpOnly，30 天）。POST 同样需要 `X-Requested-With: df-admin`。

| 方法 路径 | 说明 |
| --- | --- |
| `POST /auth/magic-link` | `{"email"}` → 始终 204（不泄露邮箱是否存在）；给该邮箱发送 `/my-orders/verify?token=…`（15 分钟，一次性） |
| `POST /auth/verify` | `{"token"}` → `{"email"}` 并 Set-Cookie；无效 → 400 `invalid_token` |
| `POST /auth/logout` | 204 |
| `GET /me/orders` | 需 Cookie。`{"email","orders":[{identifier,order_number,created_at,status,total_formatted,currency,items:[{product_name,variant_name}],license_key_count,file_count}],"subscriptions":[{id,status,status_formatted,product_name,variant_name,renews_at,ends_at,billing_url}]}` |
| `GET /orders/{identifier}` | 需 Cookie（邮箱须匹配）**或** 查询参数 `signature`（收据链接）。见下 |
| `GET /billing-url` | 需 Cookie → `{"url"}`（Stripe Billing Portal） |

`GET /orders/{identifier}` 返回：

```json
{
  "identifier":"…","order_number":1042,"status":"paid","status_formatted":"Paid","created_at":"…",
  "user_name":"…","user_email":"…","currency":"USD","subtotal_formatted":"$9.99","discount_total_formatted":"$0.00","tax_formatted":"$2.00","total_formatted":"$11.99","refunded":false,
  "card_brand":"visa","card_last_four":"4242",
  "items":[{"product_name":"…","variant_name":"…","quantity":1,"price_formatted":"$9.99"}],
  "license_keys":[{"key":"…","status":"active","status_formatted":"Active","activation_limit":5,"instances_count":1,"expires_at":null,"product_name":"…","variant_name":"…"}],
  "files":[{"id":1,"name":"app.zip","extension":"zip","size_formatted":"854 KB","version":"1.0.0","product_name":"…","variant_name":"…","download_url":"/download/<uuid>?expires=…&signature=…"}],
  "links":[{"title":"Docs","url":"https://…"}],
  "subscriptions":[{"id":1,"status":"active","status_formatted":"Active","product_name":"…","renews_at":"…","ends_at":null}],
  "billing_url":"/billing?expires=…&signature=…" ,
  "invoice_url":"https://…"
}
```

`download_url`/`billing_url` 为相对路径，浏览器直接导航即可（后端 302/流式返回）。**页面路由（Next.js）**：`/my-orders`（邮箱登录 + 订单列表）、`/my-orders/verify`（魔法链接落地页）、`/my-orders/[identifier]`（订单详情，支持 `?signature=`）。

## 8. 约定补充（实现已按此执行）

**权限矩阵**

| 资源 | viewer | admin | owner |
| --- | --- | --- | --- |
| 大多数列表/详情（orders、customers、subscriptions、license-keys、products、webhooks、deliveries、stripe-events、stores、audit-logs、email-logs、imports） | 读 | 读写 | 读写 |
| `GET /settings`、`GET /api-keys`、`GET /stripe/*` 写操作、Webhook 配置等 | ✗ | ✓ | ✓ |
| `PUT /settings`、`POST/DELETE /api-keys`、`/users`、导入 LS（`POST /imports/lemonsqueezy*`） | ✗ | ✗ | ✓ |
| 自己的资料 `PATCH /me`、`POST /me/password`、`DELETE /me/stripe` | ✓ | ✓ | ✓ |

**通用**
- 校验失败 `422`（`validation_failed`，带 `fields`；设置项的 key 是点号路径，如 `smtp.port`）；创建成功 `201`；更新/动作 `200`；删除 `204`；权限/CSRF `403`；未登录 `401`；资源不存在 `404`；唯一冲突/不可删除 `409`；Stripe 上游错误 `502`（`stripe_error`）。
- 详情接口按 ID 取，**不受 `test_mode` 参数影响**（不会因切换测试模式而 404）。
- 清空可空字段：字符串用 `""` 或 `null` 均可（`PATCH /customers`、`PATCH /users` 的 `stripe_*`、`PATCH /prices` 的可空数值用 `null`）。
- `POST /me/password`：账号还没有密码（仅 Stripe 登录）时，`current_password` 可为空，用于设置首个密码。
- 所有分页列表返回 `{data, meta}`；以下返回**裸数组**：`GET /imports/{id}/logs`、`GET /webhook-events`、`GET /stripe/catalog`。
- `StripeEvent.id` 是 Stripe 事件 ID **字符串**（`evt_…`），不是数字。

**响应形状**
- `POST /orders/{id}/refund` → 更新后的 `Order`（同列表项形状）。
- `POST /license-keys` → 新建的 `LicenseKey` 详情（含 `instances`）。
- `POST /stripe/catalog/import` → `{"products":n,"variants":n,"prices":n,"skipped":n}`。
- `PUT /settings` → 与 `GET /settings` 相同的完整结构。
- `POST /imports/*` → `ImportJob`。`ImportJob.options` 永不包含 LS API Key 明文（只保留末 4 位）。
- `ImportJob.kind`：`lemonsqueezy_api` | `lemonsqueezy_files`（ZIP）| `stripe_mapping` | `stripe_auto_match`。同一 kind 同时只能运行一个（再次启动返回 409）。
- `progress`：`{stage, done, total, counts:{…}, warnings, errors?, dry_run?}`；`counts` 除各资源数量外还可能有 `invalid`、`failed`、`conflicts`（ID 被另一条本地数据占用的行数）、`overwritten`（`on_conflict=overwrite` 时被覆盖的行数）。`options.on_conflict` 回显生效的策略。`done/total` 是**当前阶段**的进度，`total=0` 表示未知。`stage` 取值：LS 导入为资源名（`stores customers products variants prices files orders order_items subscriptions subscription_items subscription_invoices license_keys license_key_instances`）；映射任务为 `customers|products|subscriptions`；自动匹配为 `customers products prices subscriptions`。
- 上传文件：表单里的 `name` 若不带扩展名，会自动补上传文件的扩展名。
