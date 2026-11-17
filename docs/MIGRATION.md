# 从 Lemon Squeezy 迁移

> 时间线（官方）：2026-11-16 停止新用户；2026-12-02 迁移工具上线；建议最晚 2027-01 中旬开始迁；**2027-03-31 停止处理交易（含订阅续费），不会延期**。
> LS 官方的迁移工具只带走客户、商品和卡类订阅；**订单历史、License Key、文件、下载权限都要自己带走**——这正是本服务的导入器做的事。

## 0. 先理解一个原则：保留原 ID

导入时 **Lemon Squeezy 的 ID 全部保留**（`license_keys.id`、`orders.id`、`customers.id`、订单 `identifier`/`order_number`、License 实例 `identifier` = `instance_id` …），
老客户端和你自己的业务系统里存的任何 ID、Key、实例 ID 在新服务上继续有效。因此：

- **请在空库（或只有导入数据的库）上做导入。** 新数据的自增 ID 会接在最大 ID 之后（PostgreSQL 导入后自动重置序列）。
- 导入器会**拒绝覆盖非 LS 来源的订单**（例如你已经用 Stripe 在新服务上成交的订单），但其他表（商品、客户…）按 ID 覆盖。LS 的 ID 是全平台共享的大数，与新服务自增的小 ID 通常不冲突，仍建议先导入、后收款。
- 重复导入是安全的：按主键幂等 upsert；本地已有的 Stripe 关联（`stripe_*`）、激活实例、下载计数不会被覆盖；本地更新时间更新的行不会被 LS 旧数据回退。

## 1. 准备

1. 部署本服务（见 README），在后台 *设置 → Stripe* 填好密钥，配置 Webhook。**先不要把线上结账切过来。**
2. 在 LS 后台创建一个 **API Key**（只读用途即可；分别在 Live / Test 模式各建一个，如需导入测试数据）。
3. 想先试一遍？用 `dfctl demo-data` 或 `docker compose` 起一个临时实例，导入时选 **Dry run**。

## 2. 导入 Lemon Squeezy 数据

后台 **迁移（Migration）** 页，三种入口：

### A. 直接从 LS API 拉取（推荐）
填 API Key → 选择资源（默认全部）→ 勾选「下载文件」→ 建议**先 Dry run**：只拉取、校验、统计，不写库、不下载。
导入器按依赖顺序处理：`stores → customers → products → variants → prices → files → orders → order-items → subscriptions → subscription-items → subscription-invoices → license-keys → license-key-instances`。

- 遵守 LS 限流（300 次/分钟），遇 429/5xx 自动退避重试；`links.next` 只会跟随到你配置的 LS 域名，API Key 不会被发往其他主机。
- **文件**：LS 的 `download_url` 只有 1 小时有效，所以导入时**立即下载**到本服务的存储（本地磁盘或 S3），保留 `identifier`、版本、排序、状态。下载失败会记警告，重跑即可补上。
- API Key **只在内存中使用**，任务记录里只保留末 4 位（`****abcd`）。

### B. 离线 ZIP（API Key 不离开你的电脑）
```bash
LEMONSQUEEZY_API_KEY=xxxxx python3 scripts/export-lemonsqueezy.py ls-export.zip --files
```
然后在后台「上传归档」。归档格式（导入器都接受）：

| 内容 | 说明 |
| --- | --- |
| `<resource>.json`（可多个，如 `orders-1.json`、`orders-2.json`，也可放在子目录） | 文件名以资源名开头：`stores customers products variants prices files orders order-items subscriptions subscription-items subscription-invoices license-keys license-key-instances`。内容可以是 JSON:API 文档（`{"data":[…]}`）、资源对象数组、或 NDJSON |
| `files/<identifier>` | 文件本体（也认 `<id>.zip`、`<id>/文件名`、`<id>-文件名`）。没有本体且勾选「下载文件」时尝试 `download_url` |
| `orders.csv` | **兜底**：LS「导出订单」的 CSV（列见 LS 帮助文档）。仅当没有 `orders.json` 时使用，按名称匹配已有变体，匹配不上的订单项记警告 |

安全限制：拒绝 zip-slip 路径，单个条目 ≤ 4 GiB，条目数 ≤ 20 万，不解压到磁盘。

### C. 只有订单 CSV
只放一个 `orders.csv` 进 ZIP 即可得到客户 + 订单 + 订单项（没有 License Key/文件——这些数据 CSV 里没有）。

**任务页**会实时显示阶段、计数、警告，并有增量日志，可随时取消。常见警告：
- `PayPal subscription(s) were imported for history only` — LS 官方也不支持把 PayPal 订阅迁到别处，这些客户需要用新支付方式重新订阅。
- `stored N bytes but Lemon Squeezy reported M` — 文件大小与 LS 记录不一致，请人工确认。
- 客户邮箱重复：合并到最小 ID，并重映射订单/订阅/Key 的客户 ID（日志有记录）。

导入**不会**触发出站 Webhook，也**不会**发邮件。

## 3. 关联 Stripe

LS 的迁移工具（Migration Hub，2026-12-02 起）会把每个 **variant 变成一个 Stripe product**，并给出「LS 客户 ID → Stripe 客户 ID」的映射报告。本服务需要这些关联，Stripe 的 Webhook 才能对上历史数据：

| 对象 | 关联字段 | 作用 |
| --- | --- | --- |
| 客户 | `customers.stripe_customer_id` | 客户门户、续费/退款事件归属到老客户 |
| 变体 / 价格 | `variants.stripe_product_id`、`prices.stripe_price_id` | 新订单按 Stripe 价格识别到变体，从而发 Key/文件 |
| 订阅 | `subscriptions.stripe_subscription_id` | 续费、取消、过期事件作用到历史订阅及其 License Key |

三种方式（可组合）：

1. **上传映射 CSV**（后台 → 迁移 → 步骤 2，选类型：客户 / 商品 / 订阅）。表头自动识别：按关键词（`lemon`、`ls`、`customer`、`stripe`、`product`、`price`、`subscription`…）**和**取值形态（`cus_…` `prod_…` `price_…` `sub_…`）；逗号 / 分号 / Tab 分隔、BOM、无表头都可以；客户可退回到按邮箱匹配。结果统计：matched / unchanged / unknown_ls_id / invalid_rows / conflicts，前 50 个问题写入日志。
2. **自动匹配**（步骤 3）：客户按邮箱在 Stripe 里查（多个取最早创建的并警告）；商品按 metadata（`variant_id`/`ls_variant_id`）→ 名称（`变体名`、`商品 - 变体`、`商品 (变体)`、`商品 — 变体`，忽略大小写）→ 价格（金额 + 周期 + 币种）；订阅按「客户 + 价格/商品」匹配后立即从 Stripe 同步一次状态。
3. **手工**：变体页「关联 Stripe」、订阅页「关联 Stripe 订阅」、客户页改 `stripe_customer_id`。

剩下没对上的，用 **下载缺口报告**（`GET /admin-api/imports/mapping-report.csv`）逐项处理：列出缺 Stripe ID 的客户/变体/价格/未过期订阅（PayPal 订阅会标注「cannot migrate to Stripe」）。

> 注意：自动匹配不区分测试/正式模式——不要把 LS 的测试数据与 Stripe 的正式客户混配（导入时请按 Live/Test 分别进行）。

## 4. 切换

1. **License API**：把客户端/服务端里的 `https://api.lemonsqueezy.com` 换成 `https://<你的域名>`。路径、参数、响应、错误文案不变；Key 与实例 ID 沿用。
2. **JSON:API 与 SDK**：同样只换 base URL，API Key 换成后台「API 密钥」里新建的（`dfk_live_…`）。
3. **Webhook**：在后台 *Webhook* 新建端点（事件、签名密钥自己定，6–40 位）。签名算法、头和 payload 与 LS 一致，**验签代码无需改**。用「发送测试」和投递日志确认。
4. **下单入口**：用商品的 `buy_now_url`，或把 `POST /v1/checkouts` 的调用指向新域名（`checkout_data.custom`、`product_options.redirect_url`、`custom_price` 等沿用）。LS 的结账链接/覆盖层不会自动跳转，需要你自己替换；旧链接请统一改掉。
5. **客户入口**：`/my-orders`（邮箱魔法链接登录）。把旧的 LS My Orders 链接通知客户更新；收据邮件里已带新链接。
6. **Stripe**：订阅迁移走 LS 迁移工具（至少提前 14 天通知客户）；之后在 Stripe 里订阅的续费事件会自动更新本服务里的订阅与 License Key。

## 5. 验收清单

- [ ] 抽查老客户的 Key：`POST /v1/licenses/validate`（带 `instance_id`）→ `valid:true`，`activation_usage` 与 LS 一致。
- [ ] 新激活、超限、停用行为与 LS 一致（`This license key has reached the activation limit.`）。
- [ ] 订单页（`/my-orders/<identifier>?signature=…`）能看到 Key、文件下载（点开能下到正确字节）、链接。
- [ ] 后台「缺口报告」为空，或剩余项都已知悉（PayPal 订阅等）。
- [ ] Stripe 测试环境下：新购 → 订单/Key/收据邮件/出站 Webhook；续费；取消 → Key 到期；退款 → Key 禁用。
- [ ] Stripe → 设置 → Webhook 密钥已配置，后台「Stripe 事件」无 `failed`。
- [ ] 存储已备份（本地 `DATA_DIR/files` 或 S3 桶）+ 数据库备份。

## 6. 回滚与保险

- LS 在 2027-03-31 前仍可继续使用；导入是只读拉取，不会修改 LS 数据。
- 在正式切流前，可反复 Dry run / 重跑导入；有问题可在空库重来。
- 保留 LS 的订单导出 CSV 与税务文件（2027-03-31 后无法再访问）。
- 出站 Webhook 失败会保留投递记录，可在后台逐条重投；Stripe 事件处理失败可在 Stripe 页「重放」。
