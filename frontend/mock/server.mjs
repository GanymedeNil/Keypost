// Mock backend for the Keypost admin + storefront frontend.
// Implements docs/ADMIN_API.md with in-memory fixtures. Zero dependencies: `node mock/server.mjs`.
import http from 'node:http';
import { buildDb, WEBHOOK_EVENTS, STRIPE_REQUIRED_EVENTS } from './fixtures.mjs';
import {
  ApiError, E, DAY, iso, isoDate, money, humanSize, capitalize, slugify, sleep, randomUuid, randomHex,
  paginate, sortList, matchQ, toCsv, readJson, readMultipart,
} from './lib.mjs';

const PORT = Number(process.env.PORT || 8080);
const PUBLIC_URL = process.env.PUBLIC_URL || 'http://localhost:3000';
const LATENCY = process.env.MOCK_LATENCY !== '0';
const STRIPE_LOGIN = process.env.MOCK_STRIPE_LOGIN !== '0';
const STRIPE_LOGIN_OK = process.env.MOCK_STRIPE_LOGIN_OK === '1';
const SETUP_TOKEN = 'setup-token-123';
const DEMO_PASSWORD = 'password1234';
const CUSTOMER_EMAIL = 'customer@example.com';
const BILLING_PORTAL = 'https://billing.stripe.com/p/session/mock';

const db = buildDb({ storeCount: process.env.MOCK_STORES === '1' ? 1 : 2 });
let setupRequired = process.env.MOCK_SETUP === '1';
if (setupRequired) {
  db.users.length = 0;
  db.passwords.clear();
}
let webhookEndpointExists = false;

const sessions = new Map(); // sid -> user id
const customerSessions = new Map(); // sid -> email

const RANK = { viewer: 1, admin: 2, owner: 3 };
const WRITE = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);
const PAIDISH = ['paid', 'partial_refund', 'refunded'];

// ===============================================================================================
// Response helpers
// ===============================================================================================
class Out {
  constructor(status, body, headers = {}) {
    this.status = status;
    this.body = body;
    this.headers = headers;
  }
}
const stripInternal = (k, v) => (typeof k === 'string' && k.startsWith('_') ? undefined : v);
const json = (data, status = 200, headers = {}) =>
  new Out(status, JSON.stringify(data, stripInternal), { 'content-type': 'application/json; charset=utf-8', ...headers });
const created = (data) => json(data, 201);
const noContent = () => new Out(204, '');
const redirect = (location, headers = {}) => new Out(302, '', { location, ...headers });
const attachment = (filename, body, type = 'application/octet-stream') =>
  new Out(200, body, { 'content-type': type, 'content-disposition': `attachment; filename="${filename.replace(/"/g, '')}"` });

// ===============================================================================================
// Generic helpers
// ===============================================================================================
const now = () => Date.now();
const isEmail = (s) => typeof s === 'string' && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s);
const nonEmpty = (s) => typeof s === 'string' && s.trim() !== '';
const has = (o, k) => Object.prototype.hasOwnProperty.call(o, k);
const fail = (fields) => {
  if (Object.keys(fields).length) throw E.invalid(fields);
};
const pickFields = (body, keys) => Object.fromEntries(keys.filter((k) => has(body, k)).map((k) => [k, body[k]]));
const isTest = (q) => q.test_mode === 'true';
const inScope = (x, q) =>
  (x.test_mode === undefined || !!x.test_mode === isTest(q)) && (!q.store_id || x.store_id === undefined || String(x.store_id) === String(q.store_id));

function must(coll, id, label) {
  const x = db[coll].find((e) => e.id === Number(id));
  if (!x) throw E.notFound(label || coll.replace(/s$/, ''));
  return x;
}
const touch = (obj) => {
  obj.updated_at = iso(now());
  return obj;
};
const touchOrder = touch;

function list(ctx, items, { search = [], sort = '-created_at', filters = {} } = {}) {
  const q = ctx.query;
  let l = items;
  for (const [param, field] of Object.entries(filters)) {
    const wanted = q[param];
    if (wanted === undefined || wanted === '') continue;
    const set = String(wanted).split(',');
    l = l.filter((x) => set.includes(String(typeof field === 'function' ? field(x) : x[field])));
  }
  if (q.q) l = l.filter((x) => matchQ(x, q.q, search));
  return paginate(sortList(l, q.sort || sort), q);
}

function audit(ctx, action, type, id, details = {}) {
  db.auditLogs.push({
    id: db.nextId('auditLogs'), user_email: ctx.user?.email || '', action, resource_type: type, resource_id: id ?? null, details, ip: '127.0.0.1', created_at: iso(now()),
  });
}

// ===============================================================================================
// Presenters (raw record -> contract shape)
// ===============================================================================================
const productOf = (v) => db.products.find((p) => p.id === v.product_id);
const variantOf = (id) => db.variants.find((v) => v.id === id);
const storeOfProduct = (p) => p?.store_id;
const buyNowUrl = (v) => `${PUBLIC_URL}/checkout/buy/${v.slug}-${v.id}`;

function storeP(s) {
  const orders = db.orders.filter((o) => o.store_id === s.id && !o.test_mode && PAIDISH.includes(o.status));
  const recent = orders.filter((o) => Date.parse(o.created_at) >= now() - 30 * DAY);
  const sum = (arr) => arr.reduce((t, o) => t + o.total, 0);
  return { ...s, total_sales: orders.length, total_revenue: sum(orders), thirty_day_sales: recent.length, thirty_day_revenue: sum(recent) };
}

function productP(p) {
  const variants = db.variants.filter((v) => v.product_id === p.id);
  const prices = db.prices.filter((x) => variants.some((v) => v.id === x.variant_id));
  const amounts = prices.map((x) => (x.category === 'pwyw' ? x.min_price : x.unit_price));
  return {
    ...p, buy_now_url: variants[0] ? buyNowUrl(variants[0]) : null, variants_count: variants.length,
    from_price: amounts.length ? Math.min(...amounts) : null, to_price: amounts.length ? Math.max(...amounts) : null,
  };
}
const productDetailP = (p) => ({ ...productP(p), variants: db.variants.filter((v) => v.product_id === p.id).sort((a, b) => a.sort - b.sort).map(variantP) });

function variantP(v) {
  const p = productOf(v);
  return {
    ...v, product_name: p?.name || '', prices: db.prices.filter((x) => x.variant_id === v.id), files_count: db.files.filter((f) => f.variant_id === v.id).length, buy_now_url: buyNowUrl(v),
  };
}
const variantDetailP = (v) => ({ ...variantP(v), files: db.files.filter((f) => f.variant_id === v.id).sort((a, b) => a.sort - b.sort).map(fileP) });

function fileP(f) {
  const v = variantOf(f.variant_id);
  return { ...f, variant_name: v?.name || '', product_name: v ? productOf(v)?.name || '' : '', size_formatted: humanSize(f.size) };
}

function customerP(c) {
  const orders = db.orders.filter((o) => o.customer_id === c.id && PAIDISH.includes(o.status));
  const subs = db.subscriptions.filter((s) => s.customer_id === c.id);
  return {
    ...c, total_revenue: orders.reduce((t, o) => t + o.total - o.refunded_amount, 0), mrr: subs.filter((s) => s.status === 'active').reduce((t, s) => t + monthlyValue(s), 0),
    orders_count: db.orders.filter((o) => o.customer_id === c.id).length, subscriptions_count: subs.length, license_keys_count: db.licenseKeys.filter((k) => k.customer_id === c.id).length,
  };
}
const customerDetailP = (c) => ({
  ...customerP(c),
  orders: db.orders.filter((o) => o.customer_id === c.id).sort((a, b) => b.created_at.localeCompare(a.created_at)).map(orderP),
  subscriptions: db.subscriptions.filter((s) => s.customer_id === c.id).map(subscriptionP),
  license_keys: db.licenseKeys.filter((k) => k.customer_id === c.id).map(licenseKeyP),
});

function monthlyValue(sub) {
  const price = db.prices.find((p) => p.id === sub.price_id);
  if (!price) return 0;
  const n = price.renewal_interval_quantity || 1;
  const factor = { day: 30, week: 4.345, month: 1, year: 1 / 12 }[price.renewal_interval_unit] || 1;
  return Math.round((price.unit_price * (sub.quantity || 1) * factor) / n);
}

function orderP(o) {
  const items = db.orderItems.filter((i) => i.order_id === o.id);
  const first = items[0];
  return {
    ...o, receipt_url: `${PUBLIC_URL}/my-orders/${o.identifier}?signature=${randomHex(16)}`,
    first_item: first ? { product_name: first.product_name, variant_name: first.variant_name, price: first.price, quantity: first.quantity } : null,
  };
}
function orderFiles(o) {
  if (!['paid', 'partial_refund'].includes(o.status)) return [];
  const variantIds = db.orderItems.filter((i) => i.order_id === o.id).map((i) => i.variant_id);
  return db.files.filter((f) => variantIds.includes(f.variant_id) && f.status === 'published');
}
const orderDetailP = (o) => ({
  ...orderP(o),
  items: db.orderItems.filter((i) => i.order_id === o.id),
  license_keys: db.licenseKeys.filter((k) => k.order_id === o.id).map(licenseKeyP),
  subscriptions: db.subscriptions.filter((s) => s.order_id === o.id).map(subscriptionP),
  files: orderFiles(o).map(fileP),
  emails: db.emailLogs.filter((e) => e.order_id === o.id).sort((a, b) => b.created_at.localeCompare(a.created_at)),
  webhook_deliveries: db.deliveries.filter((d) => d.resource_type === 'orders' && d.resource_id === o.id).sort((a, b) => b.created_at.localeCompare(a.created_at)).map(deliveryP),
});

const subscriptionP = (s) => ({ ...s, status_formatted: s.status === 'on_trial' ? 'On trial' : capitalize(s.status) });
const subscriptionDetailP = (s) => ({
  ...subscriptionP(s),
  invoices: db.invoices.filter((i) => i.subscription_id === s.id).sort((a, b) => b.created_at.localeCompare(a.created_at)),
  license_keys: db.licenseKeys.filter((k) => k.subscription_id === s.id).map(licenseKeyP),
});

function keyStatus(k) {
  if (k.disabled) return 'disabled';
  if (k.expires_at && Date.parse(k.expires_at) < now()) return 'expired';
  return db.instances.some((i) => i.license_key_id === k.id) ? 'active' : 'inactive';
}
function licenseKeyP(k) {
  const status = keyStatus(k);
  return { ...k, instances_count: db.instances.filter((i) => i.license_key_id === k.id).length, status, status_formatted: capitalize(status) };
}
const licenseKeyDetailP = (k) => ({ ...licenseKeyP(k), instances: db.instances.filter((i) => i.license_key_id === k.id) });

function webhookP(w) {
  const ds = db.deliveries.filter((d) => d.webhook_id === w.id);
  const recent = ds.filter((d) => Date.parse(d.created_at) >= now() - DAY);
  const sent = ds.filter((d) => d.status !== 'pending').map((d) => d.delivered_at || d.created_at).sort();
  return {
    ...w, last_sent_at: sent.length ? sent[sent.length - 1] : null,
    stats: { success_24h: recent.filter((d) => d.status === 'success').length, failed_24h: recent.filter((d) => d.status === 'failed').length },
  };
}
const deliveryP = (d) => ({ ...d, webhook_url: db.webhooks.find((w) => w.id === d.webhook_id)?.url || d.webhook_url });
const deliveryDetailP = (d) => ({ ...deliveryP(d), payload: d._payload });
const stripeEventP = (e) => ({ ...e });
const apiKeyP = (k) => ({ ...k });

// ===============================================================================================
// Route table
// ===============================================================================================
const routes = [];
function route(method, pattern, handler, opts = {}) {
  const keys = [];
  const re = new RegExp('^' + pattern.replace(/:(\w+)/g, (_, k) => (keys.push(k), '([^/]+)')) + '/?$');
  routes.push({ method, pattern, re, keys, handler, opts });
}
const get = (p, h, o) => route('GET', p, h, o);
const post = (p, h, o) => route('POST', p, h, o);
const patch = (p, h, o) => route('PATCH', p, h, o);
const put = (p, h, o) => route('PUT', p, h, o);
const del = (p, h, o) => route('DELETE', p, h, o);
const PUBLIC = { public: true };
const A = '/admin-api';
const S = '/storefront-api';

// ------------------------------------------------------------------------------------------ auth
function startSession(user, ctx) {
  const sid = randomHex(40);
  sessions.set(sid, user.id);
  user.last_login_at = iso(now());
  void ctx;
  return `df_admin=${sid}; Path=/; HttpOnly; SameSite=Lax; Max-Age=604800`;
}

get(`${A}/auth/config`, () => json({ password_login: true, stripe_login: STRIPE_LOGIN, setup_required: setupRequired }), PUBLIC);

post(`${A}/auth/setup`, (ctx) => {
  if (!setupRequired) throw E.conflict('Setup has already been completed');
  const b = ctx.body;
  if (b.setup_token !== SETUP_TOKEN) throw E.forbidden('Invalid setup token (see the server start-up log)');
  const f = {};
  if (!isEmail(b.email)) f.email = 'is invalid';
  if (!nonEmpty(b.name)) f.name = 'is required';
  if (typeof b.password !== 'string' || b.password.length < 10) f.password = 'must be at least 10 characters';
  fail(f);
  const user = {
    id: db.nextId('users'), email: b.email, name: b.name, role: 'owner', stripe_account_id: null, stripe_account_label: null, disabled: false, has_password: true,
    last_login_at: iso(now()), created_at: iso(now()),
  };
  db.users.push(user);
  db.passwords.set(user.id, b.password);
  setupRequired = false;
  return json({ user }, 200, { 'set-cookie': startSession(user, ctx) });
}, PUBLIC);

post(`${A}/auth/login`, (ctx) => {
  const { email, password } = ctx.body;
  const f = {};
  if (!nonEmpty(email)) f.email = 'is required';
  if (!nonEmpty(password)) f.password = 'is required';
  fail(f);
  const user = db.users.find((u) => u.email.toLowerCase() === String(email).toLowerCase());
  if (!user || db.passwords.get(user.id) !== password) throw new ApiError(401, 'invalid_credentials', 'Invalid email or password');
  if (user.disabled) throw E.forbidden('This account has been disabled');
  return json({ user }, 200, { 'set-cookie': startSession(user, ctx) });
}, PUBLIC);

post(`${A}/auth/logout`, (ctx) => {
  if (ctx.sid) sessions.delete(ctx.sid);
  return new Out(204, '', { 'set-cookie': 'df_admin=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0' });
}, PUBLIC);

get(`${A}/auth/stripe/start`, (ctx) => {
  const mode = ctx.query.mode === 'link' ? 'link' : 'login';
  if (mode === 'link') {
    if (!ctx.user) throw E.unauth();
    ctx.user.stripe_account_id = 'acct_1MockLinked99';
    ctx.user.stripe_account_label = 'Mock Linked Account (acct_1MockLinked99)';
    return redirect('/admin/settings/profile?linked=1');
  }
  if (STRIPE_LOGIN_OK) {
    const user = db.users.find((u) => u.role === 'owner');
    if (user) return redirect('/admin', { 'set-cookie': startSession(user, ctx) });
  }
  return redirect('/admin/login?error=stripe_denied');
}, PUBLIC);

get(`${A}/auth/stripe/callback`, () => redirect('/admin/login?error=stripe_state'), PUBLIC);

get(`${A}/me`, (ctx) => json(ctx.user));
patch(`${A}/me`, (ctx) => {
  fail(nonEmpty(ctx.body.name) ? {} : { name: 'is required' });
  ctx.user.name = ctx.body.name.trim();
  return json(ctx.user);
}, { write: 'viewer' });
post(`${A}/me/password`, (ctx) => {
  const { current_password: cur, new_password: next } = ctx.body;
  const f = {};
  if (db.passwords.get(ctx.user.id) !== cur) f.current_password = 'is incorrect';
  if (typeof next !== 'string' || next.length < 10) f.new_password = 'must be at least 10 characters';
  fail(f);
  db.passwords.set(ctx.user.id, next);
  ctx.user.has_password = true;
  return noContent();
}, { write: 'viewer' });
del(`${A}/me/stripe`, (ctx) => {
  if (!ctx.user.has_password) throw E.conflict('Set a password before unlinking your Stripe account');
  ctx.user.stripe_account_id = null;
  ctx.user.stripe_account_label = null;
  return noContent();
}, { write: 'viewer' });

// ------------------------------------------------------------------------------------------ dashboard
get(`${A}/dashboard`, (ctx) => {
  const q = ctx.query;
  const days = Math.min(365, Math.max(1, parseInt(q.days, 10) || 30));
  const t = now();
  const from = t - days * DAY;
  const prevFrom = from - days * DAY;
  const orders = db.orders.filter((o) => inScope(o, q));
  const paid = orders.filter((o) => PAIDISH.includes(o.status));
  const cur = paid.filter((o) => Date.parse(o.created_at) >= from);
  const prev = paid.filter((o) => Date.parse(o.created_at) >= prevFrom && Date.parse(o.created_at) < from);
  const sum = (arr, k = 'total') => arr.reduce((s, o) => s + o[k], 0);
  const subs = db.subscriptions.filter((s) => inScope(s, q));
  const keys = db.licenseKeys.filter((k) => inScope(k, q)).map(licenseKeyP);
  const files = db.files.filter((f) => {
    const p = productOf(variantOf(f.variant_id));
    return inScope({ test_mode: f.test_mode, store_id: p?.store_id }, q);
  });
  const series = [];
  for (let i = 0; i < days; i++) {
    const date = isoDate(t - (days - 1 - i) * DAY);
    const day = cur.filter((o) => o.created_at.slice(0, 10) === date);
    series.push({
      date, revenue: sum(day), orders: day.length,
      refunds: orders.filter((o) => o.refunded_at && o.refunded_at.slice(0, 10) === date).reduce((s, o) => s + o.refunded_amount, 0),
    });
  }
  const events = [
    ...db.stripeEvents.filter((e) => e.livemode === !isTest(q)).map((e) => ({ kind: 'stripe_event', title: e.type, status: e.status, at: e.received_at })),
    ...db.deliveries.filter((d) => !!db.webhooks.find((w) => w.id === d.webhook_id)?.test_mode === isTest(q)).map((d) => ({ kind: 'webhook_delivery', title: d.event_name, status: d.status, at: d.created_at })),
    ...db.imports.map((j) => ({ kind: 'import', title: j.kind, status: j.status, at: j.created_at })),
  ].sort((a, b) => b.at.localeCompare(a.at)).slice(0, 10);
  return json({
    currency: 'USD',
    totals: {
      revenue: sum(cur), orders: cur.length, refunded: sum(cur, 'refunded_amount'),
      customers: db.customers.filter((c) => inScope(c, q)).length,
      active_subscriptions: subs.filter((s) => ['active', 'on_trial'].includes(s.status)).length,
      mrr: subs.filter((s) => ['active', 'on_trial', 'past_due'].includes(s.status)).reduce((s, x) => s + monthlyValue(x), 0),
      license_keys: keys.length, active_license_keys: keys.filter((k) => k.status === 'active').length,
      downloads: files.reduce((s, f) => s + f.downloads_count, 0),
    },
    previous: { revenue: sum(prev), orders: prev.length },
    series,
    recent_orders: [...orders].sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, 5).map(orderP),
    recent_events: events,
    health: {
      stripe_configured: db.settings.stripe.secret_key.set, stripe_webhook_secret_set: db.settings.stripe.webhook_secret.set, smtp_configured: !!db.settings.smtp.host,
      failed_stripe_events: db.stripeEvents.filter((e) => e.status === 'failed').length,
      failed_webhook_deliveries: db.deliveries.filter((d) => d.status === 'failed').length,
      pending_webhook_deliveries: db.deliveries.filter((d) => d.status === 'pending').length,
      storage_driver: 'local',
    },
  });
});

// ------------------------------------------------------------------------------------------ stores
get(`${A}/stores`, (ctx) => json(list(ctx, db.stores.map(storeP), { search: ['name', 'slug', 'domain'], sort: 'id' })));
post(`${A}/stores`, (ctx) => {
  const b = ctx.body;
  const f = {};
  if (!nonEmpty(b.name)) f.name = 'is required';
  if (b.support_email && !isEmail(b.support_email)) f.support_email = 'is invalid';
  const slug = nonEmpty(b.slug) ? b.slug : slugify(b.name);
  if (db.stores.some((s) => s.slug === slug)) f.slug = 'has already been taken';
  fail(f);
  const s = {
    id: db.nextId('stores'), name: b.name, slug, domain: b.domain || '', url: b.url || '', avatar_url: b.avatar_url || '', plan: 'fresh', country: b.country || 'US', currency: b.currency || 'USD',
    support_email: b.support_email || '', email_from_name: b.email_from_name || b.name, receipt_subject: b.receipt_subject || '', receipt_intro: b.receipt_intro || '',
    stripe_account_id: null, total_sales: 0, total_revenue: 0, thirty_day_sales: 0, thirty_day_revenue: 0, created_at: iso(now()), updated_at: iso(now()),
  };
  db.stores.push(s);
  return created(storeP(s));
});
get(`${A}/stores/:id`, (ctx) => json(storeP(must('stores', ctx.params.id, 'store'))));
patch(`${A}/stores/:id`, (ctx) => {
  const s = must('stores', ctx.params.id, 'store');
  const b = ctx.body;
  const f = {};
  if (has(b, 'name') && !nonEmpty(b.name)) f.name = 'is required';
  if (b.support_email && !isEmail(b.support_email)) f.support_email = 'is invalid';
  if (has(b, 'slug') && db.stores.some((x) => x.slug === b.slug && x.id !== s.id)) f.slug = 'has already been taken';
  fail(f);
  Object.assign(s, pickFields(b, ['name', 'slug', 'domain', 'url', 'avatar_url', 'country', 'currency', 'support_email', 'email_from_name', 'receipt_subject', 'receipt_intro']));
  return json(storeP(touch(s)));
});
del(`${A}/stores/:id`, (ctx) => {
  const s = must('stores', ctx.params.id, 'store');
  if (db.products.some((p) => p.store_id === s.id) || db.orders.some((o) => o.store_id === s.id)) throw E.conflict('Store still has products or orders and cannot be deleted');
  db.stores.splice(db.stores.indexOf(s), 1);
  return noContent();
});

// ------------------------------------------------------------------------------------------ products
const PRODUCT_FIELDS = ['store_id', 'name', 'slug', 'description', 'status', 'thumb_url', 'large_thumb_url', 'stripe_product_id', 'test_mode'];
get(`${A}/products`, (ctx) => json(list(ctx, db.products.filter((p) => inScope(p, ctx.query)).map(productP), { search: ['name', 'slug', 'description'], filters: { status: 'status' } })));
post(`${A}/products`, (ctx) => {
  const b = ctx.body;
  const f = {};
  if (!nonEmpty(b.name)) f.name = 'is required';
  if (has(b, 'status') && !['draft', 'published'].includes(b.status)) f.status = 'must be draft or published';
  const storeId = has(b, 'store_id') ? Number(b.store_id) : db.stores[0]?.id;
  if (!db.stores.some((s) => s.id === storeId)) f.store_id = 'does not exist';
  fail(f);
  const p = {
    id: db.nextId('products'), store_id: storeId, name: b.name.trim(), slug: nonEmpty(b.slug) ? b.slug : slugify(b.name), description: b.description || '', status: b.status || 'draft',
    thumb_url: b.thumb_url || '', large_thumb_url: b.large_thumb_url || '', stripe_product_id: b.stripe_product_id || null, test_mode: !!b.test_mode, created_at: iso(now()), updated_at: iso(now()),
  };
  db.products.push(p);
  return created(productDetailP(p));
});
get(`${A}/products/:id`, (ctx) => json(productDetailP(must('products', ctx.params.id, 'product'))));
patch(`${A}/products/:id`, (ctx) => {
  const p = must('products', ctx.params.id, 'product');
  const b = ctx.body;
  const f = {};
  if (has(b, 'name') && !nonEmpty(b.name)) f.name = 'is required';
  if (has(b, 'status') && !['draft', 'published'].includes(b.status)) f.status = 'must be draft or published';
  fail(f);
  Object.assign(p, pickFields(b, PRODUCT_FIELDS));
  if (has(b, 'test_mode')) db.variants.filter((v) => v.product_id === p.id).forEach((v) => (v.test_mode = !!b.test_mode));
  return json(productDetailP(touch(p)));
});
del(`${A}/products/:id`, (ctx) => {
  const p = must('products', ctx.params.id, 'product');
  if (db.orderItems.some((i) => i.product_id === p.id)) throw E.conflict('Product is referenced by existing orders and cannot be deleted');
  const vids = db.variants.filter((v) => v.product_id === p.id).map((v) => v.id);
  db.files = db.files.filter((f) => !vids.includes(f.variant_id));
  db.prices = db.prices.filter((x) => !vids.includes(x.variant_id));
  db.variants = db.variants.filter((v) => v.product_id !== p.id);
  db.products.splice(db.products.indexOf(p), 1);
  return noContent();
});

// ------------------------------------------------------------------------------------------ variants / prices
const VARIANT_FIELDS = [
  'name', 'slug', 'description', 'links', 'sort', 'status', 'has_license_keys', 'license_activation_limit', 'is_license_limit_unlimited', 'license_length_value',
  'license_length_unit', 'is_license_length_unlimited', 'stripe_product_id',
];
function validateVariant(b, creating) {
  const f = {};
  if (creating && !nonEmpty(b.name)) f.name = 'is required';
  if (has(b, 'name') && !nonEmpty(b.name)) f.name = 'is required';
  if (has(b, 'status') && !['pending', 'draft', 'published'].includes(b.status)) f.status = 'must be pending, draft or published';
  if (has(b, 'license_length_unit') && !['days', 'months', 'years'].includes(b.license_length_unit)) f.license_length_unit = 'must be days, months or years';
  if (has(b, 'links')) {
    if (!Array.isArray(b.links) || b.links.some((l) => !nonEmpty(l?.title) || !/^https?:\/\//.test(l?.url || ''))) f.links = 'each link needs a title and an http(s) url';
  }
  return f;
}
const PRICE_FIELDS = [
  'category', 'scheme', 'unit_price', 'unit_price_decimal', 'setup_fee_enabled', 'setup_fee', 'package_size', 'tiers', 'renewal_interval_unit', 'renewal_interval_quantity',
  'trial_interval_unit', 'trial_interval_quantity', 'min_price', 'suggested_price', 'tax_code', 'currency', 'stripe_price_id',
];
function validatePrice(b, creating) {
  const f = {};
  if (has(b, 'category') && !['one_time', 'subscription', 'lead_magnet', 'pwyw'].includes(b.category)) f.category = 'is invalid';
  if (has(b, 'scheme') && !['standard', 'package', 'graduated', 'volume'].includes(b.scheme)) f.scheme = 'is invalid';
  for (const k of ['unit_price', 'setup_fee', 'min_price', 'suggested_price']) {
    if (has(b, k) && (!Number.isInteger(b[k]) || b[k] < 0)) f[k] = 'must be a non-negative integer (cents)';
  }
  if (has(b, 'renewal_interval_unit') && b.renewal_interval_unit !== null && !['day', 'week', 'month', 'year'].includes(b.renewal_interval_unit)) f.renewal_interval_unit = 'is invalid';
  if (b.category === 'subscription' && creating && !b.renewal_interval_unit) f.renewal_interval_unit = 'is required for subscriptions';
  return f;
}
function newPrice(variantId, b) {
  const sub = b.category === 'subscription';
  const p = {
    id: db.nextId('prices'), variant_id: variantId, category: 'one_time', scheme: 'standard', unit_price: 0, unit_price_decimal: null, setup_fee_enabled: false, setup_fee: 0, package_size: 1, tiers: [],
    renewal_interval_unit: sub ? 'month' : null, renewal_interval_quantity: sub ? 1 : null, trial_interval_unit: null, trial_interval_quantity: null, min_price: 0, suggested_price: 0,
    tax_code: 'eservice', currency: 'USD', stripe_price_id: null, created_at: iso(now()), updated_at: iso(now()), ...pickFields(b, PRICE_FIELDS),
  };
  db.prices.push(p);
  return p;
}

get(`${A}/variants`, (ctx) => {
  const items = db.variants.filter((v) => inScope({ test_mode: v.test_mode, store_id: productOf(v)?.store_id }, ctx.query)).map(variantP);
  return json(list(ctx, items, { search: ['name', 'slug', 'product_name'], sort: 'sort', filters: { product_id: 'product_id', status: 'status' } }));
});
post(`${A}/variants`, (ctx) => {
  const b = ctx.body;
  const f = validateVariant(b, true);
  const product = db.products.find((p) => p.id === Number(b.product_id));
  if (!product) f.product_id = 'does not exist';
  if (b.price) Object.assign(f, Object.fromEntries(Object.entries(validatePrice(b.price, true)).map(([k, v]) => [`price.${k}`, v])));
  fail(f);
  const v = {
    id: db.nextId('variants'), product_id: product.id, name: b.name.trim(), slug: nonEmpty(b.slug) ? b.slug : slugify(b.name), description: '', links: [],
    sort: db.variants.filter((x) => x.product_id === product.id).length + 1, status: 'draft', has_license_keys: false, license_activation_limit: 1, is_license_limit_unlimited: false,
    license_length_value: 1, license_length_unit: 'years', is_license_length_unlimited: true, stripe_product_id: null, test_mode: product.test_mode,
    created_at: iso(now()), updated_at: iso(now()), ...pickFields(b, VARIANT_FIELDS),
  };
  db.variants.push(v);
  if (b.price) newPrice(v.id, b.price);
  return created(variantDetailP(v));
});
get(`${A}/variants/:id`, (ctx) => json(variantDetailP(must('variants', ctx.params.id, 'variant'))));
patch(`${A}/variants/:id`, (ctx) => {
  const v = must('variants', ctx.params.id, 'variant');
  fail(validateVariant(ctx.body, false));
  Object.assign(v, pickFields(ctx.body, VARIANT_FIELDS));
  return json(variantDetailP(touch(v)));
});
del(`${A}/variants/:id`, (ctx) => {
  const v = must('variants', ctx.params.id, 'variant');
  if (db.orderItems.some((i) => i.variant_id === v.id)) throw E.conflict('Variant is referenced by existing orders and cannot be deleted');
  db.files = db.files.filter((f) => f.variant_id !== v.id);
  db.prices = db.prices.filter((x) => x.variant_id !== v.id);
  db.variants.splice(db.variants.indexOf(v), 1);
  return noContent();
});
post(`${A}/variants/:id/link-stripe`, (ctx) => {
  const v = must('variants', ctx.params.id, 'variant');
  const { stripe_product_id: pid, stripe_price_id: prid } = ctx.body;
  const f = {};
  if (!/^prod_\w+$/.test(pid || '')) f.stripe_product_id = 'must look like prod_…';
  if (!/^price_\w+$/.test(prid || '')) f.stripe_price_id = 'must look like price_…';
  fail(f);
  v.stripe_product_id = pid;
  const price = db.prices.find((x) => x.variant_id === v.id);
  if (price) price.stripe_price_id = prid;
  return json(variantDetailP(touch(v)));
});
del(`${A}/variants/:id/link-stripe`, (ctx) => {
  const v = must('variants', ctx.params.id, 'variant');
  v.stripe_product_id = null;
  db.prices.filter((x) => x.variant_id === v.id).forEach((x) => (x.stripe_price_id = null));
  touch(v);
  return noContent();
});

get(`${A}/prices`, (ctx) => {
  const items = db.prices.filter((x) => {
    const v = variantOf(x.variant_id);
    return v && inScope({ test_mode: v.test_mode, store_id: productOf(v)?.store_id }, ctx.query);
  });
  return json(list(ctx, items, { sort: 'id', filters: { variant_id: 'variant_id', category: 'category' } }));
});
post(`${A}/prices`, (ctx) => {
  const b = ctx.body;
  const f = validatePrice(b, true);
  if (!variantOf(Number(b.variant_id))) f.variant_id = 'does not exist';
  fail(f);
  return created(newPrice(Number(b.variant_id), b));
});
get(`${A}/prices/:id`, (ctx) => json(must('prices', ctx.params.id, 'price')));
patch(`${A}/prices/:id`, (ctx) => {
  const p = must('prices', ctx.params.id, 'price');
  fail(validatePrice(ctx.body, false));
  Object.assign(p, pickFields(ctx.body, PRICE_FIELDS));
  return json(touch(p));
});
del(`${A}/prices/:id`, (ctx) => {
  const p = must('prices', ctx.params.id, 'price');
  if (db.orderItems.some((i) => i.price_id === p.id)) throw E.conflict('Price is referenced by existing orders and cannot be deleted');
  db.prices.splice(db.prices.indexOf(p), 1);
  return noContent();
});

// ------------------------------------------------------------------------------------------ files
const filesInScope = (q) =>
  db.files.filter((f) => {
    const v = variantOf(f.variant_id);
    return v && inScope({ test_mode: f.test_mode, store_id: productOf(v)?.store_id }, q);
  });
get(`${A}/files`, (ctx) => json(list(ctx, filesInScope(ctx.query).map(fileP), { search: ['name', 'identifier', 'product_name', 'variant_name'], sort: 'sort', filters: { variant_id: 'variant_id', status: 'status' } })));
get(`${A}/files/:id`, (ctx) => json(fileP(must('files', ctx.params.id, 'file'))));
patch(`${A}/files/:id`, (ctx) => {
  const f = must('files', ctx.params.id, 'file');
  const b = ctx.body;
  const errs = {};
  if (has(b, 'name') && !nonEmpty(b.name)) errs.name = 'is required';
  if (has(b, 'status') && !['draft', 'published'].includes(b.status)) errs.status = 'must be draft or published';
  fail(errs);
  Object.assign(f, pickFields(b, ['name', 'version', 'sort', 'status']));
  return json(fileP(touch(f)));
});
del(`${A}/files/:id`, (ctx) => {
  const f = must('files', ctx.params.id, 'file');
  db.files.splice(db.files.indexOf(f), 1);
  return noContent();
});
get(`${A}/files/:id/download`, (ctx) => {
  const f = must('files', ctx.params.id, 'file');
  f.downloads_count += 1;
  return attachment(`${f.name}.${f.extension}`, `Mock content of ${f.name}.${f.extension} (v${f.version})\n`);
});
post(`${A}/variants/:id/files`, (ctx) => {
  const v = must('variants', ctx.params.id, 'variant');
  const { fields, file } = ctx.form;
  const errs = {};
  if (!file || file.fieldname !== 'file') errs.file = 'is required';
  const status = fields.status || 'published';
  if (!['draft', 'published'].includes(status)) errs.status = 'must be draft or published';
  fail(errs);
  const dot = file.filename.lastIndexOf('.');
  const ext = dot > 0 ? file.filename.slice(dot + 1).toLowerCase() : '';
  const baseName = dot > 0 ? file.filename.slice(0, dot) : file.filename;
  const types = { zip: 'application/zip', pdf: 'application/pdf', png: 'image/png', jpg: 'image/jpeg', txt: 'text/plain', mp4: 'video/mp4', dmg: 'application/x-apple-diskimage' };
  const rec = {
    id: db.nextId('files'), variant_id: v.id, identifier: randomUuid(), name: nonEmpty(fields.name) ? fields.name : baseName, extension: ext, size: file.size, version: fields.version || '',
    sort: db.files.filter((f) => f.variant_id === v.id).length, status, storage_driver: 'local', content_type: types[ext] || 'application/octet-stream', sha256: randomHex(64), downloads_count: 0,
    test_mode: v.test_mode, created_at: iso(now()), updated_at: iso(now()),
  };
  db.files.push(rec);
  return created(fileP(rec));
}, { multipart: true });

// ------------------------------------------------------------------------------------------ customers
get(`${A}/customers`, (ctx) =>
  json(list(ctx, db.customers.filter((c) => inScope(c, ctx.query)).map(customerP), { search: ['name', 'email'], filters: { country: 'country' } })));
post(`${A}/customers`, (ctx) => {
  const b = ctx.body;
  const f = {};
  if (!nonEmpty(b.name)) f.name = 'is required';
  if (!isEmail(b.email)) f.email = 'is invalid';
  const storeId = has(b, 'store_id') ? Number(b.store_id) : db.stores[0]?.id;
  if (!db.stores.some((s) => s.id === storeId)) f.store_id = 'does not exist';
  if (isEmail(b.email) && db.customers.some((c) => c.store_id === storeId && c.email.toLowerCase() === b.email.toLowerCase())) f.email = 'has already been taken';
  fail(f);
  const c = {
    id: db.nextId('customers'), store_id: storeId, name: b.name.trim(), email: b.email, status: 'subscribed', city: '', region: '', country: '', stripe_customer_id: null, test_mode: !!b.test_mode,
    created_at: iso(now()), updated_at: iso(now()),
  };
  db.customers.push(c);
  return created(customerDetailP(c));
});
get(`${A}/customers/:id`, (ctx) => json(customerDetailP(must('customers', ctx.params.id, 'customer'))));
patch(`${A}/customers/:id`, (ctx) => {
  const c = must('customers', ctx.params.id, 'customer');
  const b = ctx.body;
  const f = {};
  if (has(b, 'name') && !nonEmpty(b.name)) f.name = 'is required';
  if (has(b, 'email') && !isEmail(b.email)) f.email = 'is invalid';
  if (has(b, 'stripe_customer_id') && b.stripe_customer_id && !/^cus_\w+$/.test(b.stripe_customer_id)) f.stripe_customer_id = 'must look like cus_…';
  fail(f);
  Object.assign(c, pickFields(b, ['name', 'email', 'city', 'region', 'country', 'stripe_customer_id']));
  if (!c.stripe_customer_id) c.stripe_customer_id = null;
  return json(customerDetailP(touch(c)));
});
post(`${A}/customers/:id/portal-link`, (ctx) => {
  const c = must('customers', ctx.params.id, 'customer');
  if (!c.stripe_customer_id) throw E.conflict('Customer has no Stripe customer id yet; link one first');
  return json({ url: BILLING_PORTAL });
});

// ------------------------------------------------------------------------------------------ orders
function filteredOrders(ctx) {
  const q = ctx.query;
  let items = db.orders.filter((o) => inScope(o, q));
  if (q.created_from) items = items.filter((o) => Date.parse(o.created_at) >= Date.parse(q.created_from));
  if (q.created_to) items = items.filter((o) => Date.parse(o.created_at) <= Date.parse(q.created_to));
  return items.map(orderP);
}
const ORDER_SEARCH = ['identifier', 'order_number', 'user_email', 'user_name'];
const ORDER_FILTERS = { status: 'status', customer_id: 'customer_id' };
get(`${A}/orders`, (ctx) => json(list(ctx, filteredOrders(ctx), { search: ORDER_SEARCH, filters: ORDER_FILTERS })));
get(`${A}/orders/export.csv`, (ctx) => {
  let l = filteredOrders(ctx);
  for (const [param, field] of Object.entries(ORDER_FILTERS)) if (ctx.query[param]) l = l.filter((o) => ctx.query[param].split(',').includes(String(o[field])));
  if (ctx.query.q) l = l.filter((o) => matchQ(o, ctx.query.q, ORDER_SEARCH));
  l = sortList(l, ctx.query.sort || '-created_at');
  const csv = toCsv(
    ['id', 'order_number', 'identifier', 'created_at', 'status', 'name', 'email', 'currency', 'subtotal', 'discount_total', 'tax', 'total', 'refunded_amount', 'product', 'variant', 'country', 'source', 'test_mode'],
    l.map((o) => [o.id, o.order_number, o.identifier, o.created_at, o.status, o.user_name, o.user_email, o.currency, o.subtotal, o.discount_total, o.tax, o.total, o.refunded_amount, o.first_item?.product_name, o.first_item?.variant_name, o.country, o.source, o.test_mode]),
  );
  return attachment(`orders-${isoDate(now())}.csv`, csv, 'text/csv; charset=utf-8');
});
get(`${A}/orders/:id`, (ctx) => json(orderDetailP(must('orders', ctx.params.id, 'order'))));
post(`${A}/orders/:id/refund`, (ctx) => {
  const o = must('orders', ctx.params.id, 'order');
  if (!['paid', 'partial_refund'].includes(o.status)) throw E.conflict(`Order with status "${o.status}" cannot be refunded`);
  if (!o.stripe_payment_intent) throw E.stripe('Order has no Stripe payment intent (imported from Lemon Squeezy or created manually); refund it in your payment provider');
  const remaining = o.total - o.refunded_amount;
  const hasAmount = ctx.body.amount !== undefined && ctx.body.amount !== null;
  const amount = hasAmount ? ctx.body.amount : remaining;
  if (!Number.isInteger(amount) || amount < 1 || amount > remaining) throw E.invalid({ amount: `must be an integer between 1 and ${remaining}` });
  o.refunded_amount += amount;
  o.refunded_amount_usd = o.refunded_amount;
  o.refunded = true;
  o.refunded_at = iso(now());
  o.status = o.refunded_amount >= o.total ? 'refunded' : 'partial_refund';
  if (o.status === 'refunded' && db.settings.license.disable_on_refund) db.licenseKeys.filter((k) => k.order_id === o.id).forEach((k) => (k.disabled = true));
  touchOrder(o);
  audit(ctx, 'order.refund', 'orders', o.id, { amount, reason: ctx.body.reason || null });
  return json(orderDetailP(o));
});
post(`${A}/orders/:id/resend-receipt`, (ctx) => {
  const o = must('orders', ctx.params.id, 'order');
  const to = ctx.body.email || o.user_email;
  if (!isEmail(to)) throw E.invalid({ email: 'is invalid' });
  db.emailLogs.push({ id: db.nextId('emailLogs'), order_id: o.id, to, subject: `Your receipt for order #${o.order_number}`, template: 'order_receipt', status: db.settings.smtp.host ? 'sent' : 'failed', error: db.settings.smtp.host ? '' : 'smtp: host not configured', created_at: iso(now()) });
  return noContent();
});

// ------------------------------------------------------------------------------------------ subscriptions
get(`${A}/subscriptions`, (ctx) =>
  json(list(ctx, db.subscriptions.filter((s) => inScope(s, ctx.query)).map(subscriptionP), {
    search: ['user_name', 'user_email', 'product_name', 'variant_name', 'stripe_subscription_id'], filters: { status: 'status', customer_id: 'customer_id', product_id: 'product_id' },
  })));
get(`${A}/subscriptions/:id`, (ctx) => json(subscriptionDetailP(must('subscriptions', ctx.params.id, 'subscription'))));
post(`${A}/subscriptions/:id/cancel`, (ctx) => {
  const s = must('subscriptions', ctx.params.id, 'subscription');
  if (['cancelled', 'expired'].includes(s.status)) throw E.conflict(`Subscription is already ${s.status}`);
  s.ends_at = s.renews_at || iso(now() + 30 * DAY);
  s.renews_at = null;
  s.cancelled = true;
  s.pause = null;
  s.status = 'cancelled';
  return json(subscriptionDetailP(touch(s)));
});
post(`${A}/subscriptions/:id/resume`, (ctx) => {
  const s = must('subscriptions', ctx.params.id, 'subscription');
  if (s.status === 'paused') {
    s.status = 'active';
    s.pause = null;
  } else if (s.status === 'cancelled' && s.ends_at && Date.parse(s.ends_at) > now()) {
    s.status = 'active';
    s.cancelled = false;
    s.renews_at = s.ends_at;
    s.ends_at = null;
  } else {
    throw E.conflict('Only paused subscriptions or subscriptions cancelled at period end can be resumed');
  }
  return json(subscriptionDetailP(touch(s)));
});
post(`${A}/subscriptions/:id/sync`, (ctx) => {
  const s = must('subscriptions', ctx.params.id, 'subscription');
  if (!s.stripe_subscription_id) throw E.conflict('Subscription is not linked to Stripe yet; link it first');
  return json(subscriptionDetailP(touch(s)));
});
post(`${A}/subscriptions/:id/link-stripe`, (ctx) => {
  const s = must('subscriptions', ctx.params.id, 'subscription');
  const id = ctx.body.stripe_subscription_id;
  if (!/^sub_\w+$/.test(id || '')) throw E.invalid({ stripe_subscription_id: 'must look like sub_…' });
  s.stripe_subscription_id = id;
  return json(subscriptionDetailP(touch(s)));
});
get(`${A}/subscription-invoices`, (ctx) =>
  json(list(ctx, db.invoices.filter((i) => !ctx.query.subscription_id || String(i.subscription_id) === ctx.query.subscription_id), { search: ['user_email', 'stripe_invoice_id'], filters: { status: 'status' } })));

// ------------------------------------------------------------------------------------------ license keys
const lkScope = (ctx) => db.licenseKeys.filter((k) => inScope(k, ctx.query)).map(licenseKeyP);
get(`${A}/license-keys`, (ctx) =>
  json(list(ctx, lkScope(ctx), {
    search: ['key', 'user_email', 'user_name'],
    filters: { status: 'status', order_id: 'order_id', customer_id: 'customer_id', product_id: 'product_id', variant_id: 'variant_id' },
  })));
post(`${A}/license-keys`, (ctx) => {
  const b = ctx.body;
  const f = {};
  const variant = variantOf(Number(b.variant_id));
  if (!variant) f.variant_id = 'does not exist';
  const order = b.order_id ? db.orders.find((o) => o.id === Number(b.order_id)) : null;
  if (b.order_id && !order) f.order_id = 'does not exist';
  let customer = b.customer_id ? db.customers.find((c) => c.id === Number(b.customer_id)) : null;
  if (b.customer_id && !customer) f.customer_id = 'does not exist';
  if (!order && !customer && !isEmail(b.user_email)) f.user_email = 'is required (or pass customer_id / order_id)';
  if (has(b, 'activation_limit') && b.activation_limit !== null && (!Number.isInteger(b.activation_limit) || b.activation_limit < 1)) f.activation_limit = 'must be a positive integer or null';
  if (b.expires_at && Number.isNaN(Date.parse(b.expires_at))) f.expires_at = 'is not a valid date';
  fail(f);
  if (order && !customer) customer = db.customers.find((c) => c.id === order.customer_id);
  if (!customer && isEmail(b.user_email)) customer = db.customers.find((c) => c.email.toLowerCase() === b.user_email.toLowerCase()) || null;
  const product = productOf(variant);
  const key = randomUuid();
  const k = {
    id: db.nextId('licenseKeys'), store_id: product.store_id, customer_id: customer?.id ?? null, order_id: order?.id ?? null, order_item_id: null, product_id: product.id, variant_id: variant.id, subscription_id: null,
    product_name: product.name, variant_name: variant.name, user_name: b.user_name || customer?.name || order?.user_name || '', user_email: b.user_email || customer?.email || order?.user_email || '',
    key, key_short: `XXXX-${key.slice(-8)}`,
    activation_limit: has(b, 'activation_limit') ? b.activation_limit : variant.is_license_limit_unlimited ? null : variant.license_activation_limit || 1,
    disabled: false, expires_at: b.expires_at || null, test_mode: variant.test_mode, created_at: iso(now()), updated_at: iso(now()),
  };
  db.licenseKeys.push(k);
  return created(licenseKeyDetailP(k));
});
get(`${A}/license-keys/:id`, (ctx) => json(licenseKeyDetailP(must('licenseKeys', ctx.params.id, 'license key'))));
patch(`${A}/license-keys/:id`, (ctx) => {
  const k = must('licenseKeys', ctx.params.id, 'license key');
  const b = ctx.body;
  const f = {};
  if (has(b, 'activation_limit') && b.activation_limit !== null && (!Number.isInteger(b.activation_limit) || b.activation_limit < 1)) f.activation_limit = 'must be a positive integer or null';
  if (has(b, 'expires_at') && b.expires_at !== null && Number.isNaN(Date.parse(b.expires_at))) f.expires_at = 'is not a valid date';
  if (has(b, 'disabled') && typeof b.disabled !== 'boolean') f.disabled = 'must be true or false';
  fail(f);
  Object.assign(k, pickFields(b, ['activation_limit', 'expires_at', 'disabled']));
  return json(licenseKeyDetailP(touch(k)));
});
post(`${A}/license-keys/:id/regenerate`, (ctx) => {
  const k = must('licenseKeys', ctx.params.id, 'license key');
  k.key = randomUuid();
  k.key_short = `XXXX-${k.key.slice(-8)}`;
  db.instances = db.instances.filter((i) => i.license_key_id !== k.id);
  return json(licenseKeyDetailP(touch(k)));
});
get(`${A}/license-key-instances`, (ctx) => json(list(ctx, db.instances, { search: ['name', 'identifier'], sort: 'id', filters: { license_key_id: 'license_key_id' } })));
del(`${A}/license-key-instances/:id`, (ctx) => {
  const i = must('instances', ctx.params.id, 'instance');
  db.instances.splice(db.instances.indexOf(i), 1);
  return noContent();
});

// ------------------------------------------------------------------------------------------ webhooks
get(`${A}/webhook-events`, () => json(WEBHOOK_EVENTS));
function validateWebhook(b, creating) {
  const f = {};
  if ((creating || has(b, 'url')) && !/^https?:\/\/[^\s]+$/.test(b.url || '')) f.url = 'must be a valid http(s) URL';
  if ((creating || has(b, 'events')) && (!Array.isArray(b.events) || b.events.length === 0)) f.events = 'select at least one event';
  else if (has(b, 'events') && b.events.some((e) => !WEBHOOK_EVENTS.includes(e))) f.events = 'contains unknown events';
  if (has(b, 'secret') && b.secret !== '' && b.secret !== null && (typeof b.secret !== 'string' || b.secret.length < 6 || b.secret.length > 40)) f.secret = 'must be 6-40 characters';
  return f;
}
get(`${A}/webhooks`, (ctx) => json(list(ctx, db.webhooks.filter((w) => inScope(w, ctx.query)).map(webhookP), { search: ['url'], filters: {} })));
post(`${A}/webhooks`, (ctx) => {
  const b = ctx.body;
  fail(validateWebhook(b, true));
  const w = {
    id: db.nextId('webhooks'), store_id: has(b, 'store_id') ? Number(b.store_id) : db.stores[0].id, url: b.url, events: b.events, disabled: !!b.disabled, secret_set: !!b.secret, test_mode: !!b.test_mode,
    created_at: iso(now()), updated_at: iso(now()),
  };
  db.webhooks.push(w);
  return created(webhookP(w));
});
get(`${A}/webhooks/:id`, (ctx) => json(webhookP(must('webhooks', ctx.params.id, 'webhook'))));
patch(`${A}/webhooks/:id`, (ctx) => {
  const w = must('webhooks', ctx.params.id, 'webhook');
  const b = ctx.body;
  fail(validateWebhook(b, false));
  Object.assign(w, pickFields(b, ['store_id', 'url', 'events', 'disabled', 'test_mode']));
  if (has(b, 'secret') && b.secret) w.secret_set = true;
  return json(webhookP(touch(w)));
});
del(`${A}/webhooks/:id`, (ctx) => {
  const w = must('webhooks', ctx.params.id, 'webhook');
  db.deliveries = db.deliveries.filter((d) => d.webhook_id !== w.id);
  db.webhooks.splice(db.webhooks.indexOf(w), 1);
  return noContent();
});
function attemptDelivery(d, w) {
  const failing = /fail|invalid/i.test(w.url);
  const dur = 60 + Math.floor(Math.random() * 400);
  d.attempts += 1;
  d.status = failing ? 'failed' : 'success';
  d.last_status_code = failing ? 500 : 200;
  d.last_response = failing ? 'Internal Server Error' : '{"ok":true}';
  d.last_error = '';
  d.duration_ms = dur;
  d.next_attempt_at = null;
  d.delivered_at = failing ? null : iso(now());
  return d;
}
post(`${A}/webhooks/:id/test`, (ctx) => {
  const w = must('webhooks', ctx.params.id, 'webhook');
  const order = db.orders.find((o) => !o.test_mode === !w.test_mode) || db.orders[0];
  const d = {
    id: db.nextId('deliveries'), webhook_id: w.id, webhook_url: w.url, event_name: 'order_created', resource_type: 'orders', resource_id: order.id, status: 'pending', attempts: 0,
    next_attempt_at: null, last_status_code: null, last_response: '', last_error: '', duration_ms: 0, created_at: iso(now()), delivered_at: null,
    _payload: JSON.stringify({ meta: { test_mode: w.test_mode, event_name: 'order_created', webhook_id: String(w.id) }, data: { type: 'orders', id: String(order.id), attributes: { identifier: order.identifier, total: order.total, user_email: order.user_email, test: true } } }, null, 2),
  };
  attemptDelivery(d, w);
  db.deliveries.push(d);
  return json(deliveryDetailP(d));
});
get(`${A}/webhook-deliveries`, (ctx) => {
  const items = db.deliveries.filter((d) => ctx.query.webhook_id || !!db.webhooks.find((w) => w.id === d.webhook_id)?.test_mode === isTest(ctx.query)).map(deliveryP);
  return json(list(ctx, items, { search: ['event_name', 'webhook_url', 'last_error'], filters: { webhook_id: 'webhook_id', status: 'status', event_name: 'event_name' } }));
});
get(`${A}/webhook-deliveries/:id`, (ctx) => json(deliveryDetailP(must('deliveries', ctx.params.id, 'delivery'))));
post(`${A}/webhook-deliveries/:id/redeliver`, (ctx) => {
  const d = must('deliveries', ctx.params.id, 'delivery');
  const w = db.webhooks.find((x) => x.id === d.webhook_id);
  if (!w) throw E.conflict('Webhook endpoint no longer exists');
  return json(deliveryDetailP(attemptDelivery(d, w)));
});

// ------------------------------------------------------------------------------------------ stripe events
get(`${A}/stripe-events`, (ctx) =>
  json(list(ctx, db.stripeEvents.filter((e) => e.livemode === !isTest(ctx.query)).map(stripeEventP), { search: ['type', 'account', 'error'], sort: '-received_at', filters: { status: 'status', type: 'type' } })));
get(`${A}/stripe-events/:id`, (ctx) => {
  const e = must('stripeEvents', ctx.params.id, 'stripe event');
  return json({ ...stripeEventP(e), payload: e._payload });
});
post(`${A}/stripe-events/:id/replay`, (ctx) => {
  const e = must('stripeEvents', ctx.params.id, 'stripe event');
  e.attempts += 1;
  e.status = 'processed';
  e.error = '';
  e.processed_at = iso(now());
  return json({ ...stripeEventP(e), payload: e._payload });
});

// ------------------------------------------------------------------------------------------ api keys / users
get(`${A}/api-keys`, (ctx) => json(list(ctx, db.apiKeys.map(apiKeyP), { search: ['name', 'prefix'], filters: {} })), { read: 'admin' });
post(`${A}/api-keys`, (ctx) => {
  const b = ctx.body;
  fail(nonEmpty(b.name) ? {} : { name: 'is required' });
  if (b.expires_at && Number.isNaN(Date.parse(b.expires_at))) throw E.invalid({ expires_at: 'is not a valid date' });
  const token = `dfk_${b.test_mode ? 'test' : 'live'}_${randomHex(40)}`;
  const k = {
    id: db.nextId('apiKeys'), name: b.name.trim(), prefix: token.slice(0, 13), test_mode: !!b.test_mode, owner_email: ctx.user.email, last_used_at: null, expires_at: b.expires_at || null,
    revoked_at: null, created_at: iso(now()),
  };
  db.apiKeys.push(k);
  return created({ ...k, token });
}, { read: 'admin', write: 'owner' });
del(`${A}/api-keys/:id`, (ctx) => {
  const k = must('apiKeys', ctx.params.id, 'API key');
  k.revoked_at ||= iso(now());
  return noContent();
}, { write: 'owner' });

const ROLES = ['owner', 'admin', 'viewer'];
const OWNER_ONLY = { read: 'owner', write: 'owner' };
const ownerCount = () => db.users.filter((u) => u.role === 'owner' && !u.disabled).length;
get(`${A}/users`, (ctx) => json(list(ctx, db.users, { search: ['name', 'email'], sort: 'id', filters: { role: 'role' } })), OWNER_ONLY);
post(`${A}/users`, (ctx) => {
  const b = ctx.body;
  const f = {};
  if (!isEmail(b.email)) f.email = 'is invalid';
  else if (db.users.some((u) => u.email.toLowerCase() === b.email.toLowerCase())) f.email = 'has already been taken';
  if (!nonEmpty(b.name)) f.name = 'is required';
  if (!ROLES.includes(b.role)) f.role = 'must be owner, admin or viewer';
  if (b.password && b.password.length < 10) f.password = 'must be at least 10 characters';
  if (!b.password && !b.stripe_account_id) f.password = 'is required unless a Stripe account is linked';
  fail(f);
  const u = {
    id: db.nextId('users'), email: b.email, name: b.name.trim(), role: b.role, stripe_account_id: b.stripe_account_id || null, stripe_account_label: b.stripe_account_id ? `${b.stripe_account_id}` : null,
    disabled: false, has_password: !!b.password, last_login_at: null, created_at: iso(now()),
  };
  db.users.push(u);
  if (b.password) db.passwords.set(u.id, b.password);
  return created(u);
}, OWNER_ONLY);
get(`${A}/users/:id`, (ctx) => json(must('users', ctx.params.id, 'user')), OWNER_ONLY);
patch(`${A}/users/:id`, (ctx) => {
  const u = must('users', ctx.params.id, 'user');
  const b = ctx.body;
  const f = {};
  if (has(b, 'name') && !nonEmpty(b.name)) f.name = 'is required';
  if (has(b, 'role') && !ROLES.includes(b.role)) f.role = 'must be owner, admin or viewer';
  if (b.password && b.password.length < 10) f.password = 'must be at least 10 characters';
  fail(f);
  const losesOwner = u.role === 'owner' && ((has(b, 'role') && b.role !== 'owner') || b.disabled === true);
  if (losesOwner && ownerCount() <= 1) throw E.conflict('Cannot demote or disable the last owner');
  Object.assign(u, pickFields(b, ['name', 'role', 'disabled', 'stripe_account_id']));
  if (has(b, 'stripe_account_id')) u.stripe_account_label = b.stripe_account_id || null;
  if (b.password) {
    db.passwords.set(u.id, b.password);
    u.has_password = true;
  }
  if (u.disabled) for (const [sid, uid] of sessions) if (uid === u.id) sessions.delete(sid);
  return json(u);
}, OWNER_ONLY);
del(`${A}/users/:id`, (ctx) => {
  const u = must('users', ctx.params.id, 'user');
  if (u.role === 'owner' && ownerCount() <= 1) throw E.conflict('Cannot delete the last owner');
  db.users.splice(db.users.indexOf(u), 1);
  db.passwords.delete(u.id);
  for (const [sid, uid] of sessions) if (uid === u.id) sessions.delete(sid);
  return noContent();
}, OWNER_ONLY);

// ------------------------------------------------------------------------------------------ audit / email logs
get(`${A}/audit-logs`, (ctx) => json(list(ctx, db.auditLogs, { search: ['action', 'user_email', 'resource_type', 'ip'], filters: { action: 'action', user_email: 'user_email', resource_type: 'resource_type' } })));
get(`${A}/email-logs`, (ctx) => json(list(ctx, db.emailLogs, { search: ['to', 'subject', 'template'], filters: { status: 'status', order_id: 'order_id' } })));
function sendTestEmail(ctx, template) {
  const to = ctx.body.to;
  if (!isEmail(to)) throw E.invalid({ to: 'is invalid' });
  const smtp = db.settings.smtp;
  if (!smtp.host) throw E.conflict('SMTP is not configured. Set a host in Settings → Email first.');
  const ok = !/fail/i.test(smtp.host);
  db.emailLogs.push({ id: db.nextId('emailLogs'), order_id: null, to, subject: 'Keypost test email', template, status: ok ? 'sent' : 'failed', error: ok ? '' : 'smtp: connection refused', created_at: iso(now()) });
  if (!ok) throw new ApiError(502, 'internal', `SMTP delivery failed: connection to ${smtp.host}:${smtp.port} refused`);
  return noContent();
}
post(`${A}/email-logs/test`, (ctx) => sendTestEmail(ctx, 'test'));
post(`${A}/settings/smtp/test`, (ctx) => sendTestEmail(ctx, 'smtp_test'));

// ------------------------------------------------------------------------------------------ settings
const SECRETS = { stripe: ['secret_key', 'webhook_secret', 'oauth_secret_key'], smtp: ['password'] };
const hintOf = (s) => `${/^[a-z]+_(?:live_|test_)?/i.exec(s)?.[0] ?? s.slice(0, 4)}…${s.slice(-4)}`;
get(`${A}/settings`, () => json(db.settings), { read: 'admin' });
put(`${A}/settings`, (ctx) => {
  const b = ctx.body;
  const f = {};
  const st = db.settings;
  if (b.stripe?.login_mode !== undefined && !['allowlist', 'auto'].includes(b.stripe.login_mode)) f['stripe.login_mode'] = 'must be allowlist or auto';
  if (b.stripe?.login_auto_role !== undefined && !['admin', 'viewer'].includes(b.stripe.login_auto_role)) f['stripe.login_auto_role'] = 'must be admin or viewer';
  if (b.stripe?.login_accounts !== undefined && !Array.isArray(b.stripe.login_accounts)) f['stripe.login_accounts'] = 'must be a list';
  if (b.smtp?.port !== undefined && (!Number.isInteger(b.smtp.port) || b.smtp.port < 1 || b.smtp.port > 65535)) f['smtp.port'] = 'must be a valid port';
  if (b.smtp?.security !== undefined && !['none', 'starttls', 'tls'].includes(b.smtp.security)) f['smtp.security'] = 'must be none, starttls or tls';
  if (b.smtp?.from && !isEmail(String(b.smtp.from).replace(/^.*<(.+)>$/, '$1'))) f['smtp.from'] = 'is invalid';
  if (b.download?.limit_per_day !== undefined && (!Number.isInteger(b.download.limit_per_day) || b.download.limit_per_day < 0)) f['download.limit_per_day'] = 'must be >= 0';
  if (b.download?.url_ttl_seconds !== undefined && (!Number.isInteger(b.download.url_ttl_seconds) || b.download.url_ttl_seconds < 60)) f['download.url_ttl_seconds'] = 'must be >= 60';
  fail(f);
  for (const group of ['stripe', 'smtp', 'license', 'download', 'checkout']) {
    const patchGroup = b[group];
    if (!patchGroup || typeof patchGroup !== 'object') continue;
    for (const [key, value] of Object.entries(patchGroup)) {
      if (!(key in st[group])) continue;
      if ((SECRETS[group] || []).includes(key)) {
        if (typeof value !== 'string') continue;
        st[group][key] = value === '' ? { set: false } : { set: true, source: 'db', hint: hintOf(value) };
      } else {
        st[group][key] = value;
      }
    }
  }
  audit(ctx, 'settings.update', 'settings', null, { groups: Object.keys(b) });
  return json(db.settings);
}, { write: 'owner' });

// ------------------------------------------------------------------------------------------ stripe integration
get(`${A}/stripe/status`, () => {
  const configured = db.settings.stripe.secret_key.set;
  return json({
    configured, livemode: configured && !/test/.test(db.settings.stripe.secret_key.hint || ''),
    account: configured ? { id: 'acct_1MockOwner01', business_name: 'Acme Software Inc.', country: 'US', email: 'billing@acme-software.example.com' } : null,
    webhook_secret_set: db.settings.stripe.webhook_secret.set, webhook_url: db.settings.urls.stripe_webhook_url, required_events: STRIPE_REQUIRED_EVENTS,
    ...(configured ? {} : { error: 'Stripe secret key is not configured' }),
  });
});
post(`${A}/stripe/webhook-endpoint`, () => {
  if (!db.settings.stripe.secret_key.set) throw E.stripe('Stripe secret key is not configured');
  const createdNow = !webhookEndpointExists;
  webhookEndpointExists = true;
  db.settings.stripe.webhook_secret = { set: true, source: 'db', hint: `whsec_…${randomHex(4)}` };
  return json({ id: 'we_1MockEndpoint', url: db.settings.urls.stripe_webhook_url, events: STRIPE_REQUIRED_EVENTS, created: createdNow });
});
get(`${A}/stripe/catalog`, () => {
  if (!db.settings.stripe.secret_key.set) throw E.stripe('Stripe secret key is not configured');
  return json(db.catalog.map((p) => ({ ...p, mapped_variant_id: db.variants.find((v) => v.stripe_product_id === p.id)?.id ?? null })));
});
post(`${A}/stripe/catalog/import`, (ctx) => {
  const b = ctx.body;
  const store = db.stores.find((s) => s.id === Number(b.store_id));
  if (!store) throw E.invalid({ store_id: 'does not exist' });
  const mapped = new Set(db.variants.map((v) => v.stripe_product_id));
  const wanted = Array.isArray(b.stripe_product_ids) && b.stripe_product_ids.length ? b.stripe_product_ids : db.catalog.map((p) => p.id);
  const targets = db.catalog.filter((p) => wanted.includes(p.id) && !mapped.has(p.id));
  const imported = [];
  for (const sp of targets) {
    const p = {
      id: db.nextId('products'), store_id: store.id, name: sp.name, slug: slugify(sp.name), description: sp.description || '', status: b.publish ? 'published' : 'draft', thumb_url: '', large_thumb_url: '',
      stripe_product_id: sp.id, test_mode: false, created_at: iso(now()), updated_at: iso(now()),
    };
    db.products.push(p);
    const v = {
      id: db.nextId('variants'), product_id: p.id, name: 'Default', slug: 'default', description: '', links: [], sort: 1, status: b.publish ? 'published' : 'draft', has_license_keys: false,
      license_activation_limit: 1, is_license_limit_unlimited: false, license_length_value: 1, license_length_unit: 'years', is_license_length_unlimited: true, stripe_product_id: sp.id, test_mode: false,
      created_at: iso(now()), updated_at: iso(now()),
    };
    db.variants.push(v);
    const sprice = sp.prices.find((x) => x.active) || sp.prices[0];
    if (sprice) {
      newPrice(v.id, {
        category: sprice.recurring ? 'subscription' : 'one_time', unit_price: sprice.unit_amount, currency: sprice.currency.toUpperCase(), stripe_price_id: sprice.id,
        ...(sprice.recurring ? { renewal_interval_unit: sprice.recurring.interval, renewal_interval_quantity: sprice.recurring.interval_count } : {}),
      });
    }
    imported.push(p.id);
  }
  return json({ imported: imported.length, skipped: wanted.length - imported.length, product_ids: imported });
});

// ------------------------------------------------------------------------------------------ imports
const jobP = (j) => ({ ...j });
const jobLog = (job, level, message) => db.importLogs.push({ id: db.nextId('importLogs'), job_id: job.id, level, message, created_at: iso(now()) });
const LS_RESOURCES = ['stores', 'products', 'variants', 'prices', 'files', 'customers', 'orders', 'subscriptions', 'license_keys'];
const LS_TOTALS = () => ({
  stores: db.stores.length, products: db.products.length, variants: db.variants.length, prices: db.prices.length, files: db.files.length, customers: db.customers.length, orders: db.orders.length,
  subscriptions: db.subscriptions.length, license_keys: db.licenseKeys.length,
});
function startJob(ctx, { kind, options, stages, duration, fail: shouldFail = false, hook = null, dryRun = false }) {
  const t = now();
  const job = {
    id: db.nextId('imports'), kind, status: 'pending', options, progress: { stage: stages[0]?.[0] || 'init', done: 0, total: stages[0]?.[1] || 0, counts: {}, warnings: 0 }, error: null,
    created_by: ctx.user.email, started_at: null, finished_at: null, created_at: iso(t), _t0: t, _duration: duration, _stages: stages, _idx: -1, _fail: shouldFail, _hook: hook, _dry: dryRun, _warnTick: 0,
  };
  db.imports.push(job);
  jobLog(job, 'info', `${dryRun ? '[dry-run] ' : ''}Import queued (${kind})`);
  return job;
}
function tickJobs() {
  const t = now();
  for (const job of db.imports) {
    if (!job._t0 || !['pending', 'running'].includes(job.status)) continue;
    const elapsed = t - job._t0;
    if (elapsed < 1500) continue;
    const prefix = job._dry ? '[dry-run] ' : '';
    if (job.status === 'pending') {
      job.status = 'running';
      job.started_at = iso(t);
      jobLog(job, 'info', `${prefix}Starting import`);
    }
    const frac = Math.min(1, (elapsed - 1500) / job._duration);
    if (job._fail && elapsed >= 4500) {
      job.status = 'failed';
      job.error = 'Lemon Squeezy API rejected the API key (401 Unauthorized)';
      job.finished_at = iso(t);
      jobLog(job, 'error', job.error);
      continue;
    }
    const n = job._stages.length;
    const pos = frac * n;
    const idx = Math.min(n - 1, Math.floor(pos));
    const stageFrac = frac >= 1 ? 1 : pos - idx;
    const counts = {};
    job._stages.forEach(([name, total], i) => {
      if (i < idx) counts[name] = total;
      else if (i === idx) counts[name] = Math.round(stageFrac * total);
    });
    const [stage, total] = job._stages[idx];
    if (idx !== job._idx) {
      if (job._idx >= 0) jobLog(job, 'info', `${prefix}Finished ${job._stages[job._idx][0]}`);
      jobLog(job, 'info', `${prefix}Importing ${stage} (${total})`);
      job._idx = idx;
    }
    job.progress = { stage, done: counts[stage], total, counts, warnings: job.progress.warnings };
    jobLog(job, 'info', `${prefix}${stage}: ${counts[stage]}/${total}`);
    job._warnTick += 1;
    if (job._warnTick % 7 === 0 && total > 3) {
      job.progress.warnings += 1;
      jobLog(job, 'warn', `${prefix}${stage}: skipped a record that could not be mapped (missing required field)`);
    }
    if (frac >= 1) {
      job.status = 'completed';
      job.finished_at = iso(t);
      jobLog(job, 'info', `${prefix}Import completed${job.progress.warnings ? ` with ${job.progress.warnings} warning(s)` : ''}`);
      if (job._hook && !job._dry) job._hook(job);
    }
  }
}
setInterval(tickJobs, 1000);

const unmapped = {
  customers: () => db.customers.filter((c) => !c.stripe_customer_id && !c.test_mode),
  products: () => db.variants.filter((v) => !v.stripe_product_id && !v.test_mode),
  subscriptions: () => db.subscriptions.filter((s) => !s.stripe_subscription_id && !s.test_mode),
};
function applyMapping(kind, limit) {
  const targets = unmapped[kind]().slice(0, limit);
  for (const x of targets) {
    if (kind === 'customers') x.stripe_customer_id = `cus_Auto${randomHex(10)}`;
    else if (kind === 'products') x.stripe_product_id = `prod_Auto${randomHex(10)}`;
    else x.stripe_subscription_id = `sub_Auto${randomHex(10)}`;
    touch(x);
  }
  return targets.length;
}
const bool = (v, d = false) => (v === undefined ? d : v === true || v === 'true' || v === 'on' || v === '1');

function lsStages(resources, downloadFiles) {
  const totals = LS_TOTALS();
  const chosen = LS_RESOURCES.filter((r) => !resources || resources.includes(r)).filter((r) => r !== 'files' || downloadFiles || (resources && resources.includes('files')));
  return chosen.map((r) => [r, totals[r]]);
}
post(`${A}/imports/lemonsqueezy`, (ctx) => {
  const b = ctx.body;
  if (!nonEmpty(b.api_key)) throw E.invalid({ api_key: 'is required' });
  if (b.resources && (!Array.isArray(b.resources) || b.resources.some((r) => !LS_RESOURCES.includes(r)))) throw E.invalid({ resources: 'contains unknown resources' });
  const dryRun = bool(b.dry_run);
  const stages = lsStages(b.resources, bool(b.download_files, true));
  const job = startJob(ctx, {
    kind: 'lemonsqueezy_api', stages: stages.length ? stages : [['stores', 1]], duration: 25_000, fail: b.api_key.trim() === 'fail', dryRun,
    options: { test_mode: bool(b.test_mode), store_ids: b.store_ids || [], resources: b.resources || LS_RESOURCES, download_files: bool(b.download_files, true), dry_run: dryRun },
  });
  return created(jobP(job));
});
post(`${A}/imports/lemonsqueezy/archive`, (ctx) => {
  const { fields, file } = ctx.form;
  if (!file || file.fieldname !== 'file') throw E.invalid({ file: 'is required' });
  const downloadFiles = bool(fields.download_files);
  const job = startJob(ctx, {
    kind: 'lemonsqueezy_archive', stages: lsStages(null, downloadFiles), duration: 18_000, options: { download_files: downloadFiles, filename: file.filename, size: file.size },
  });
  return created(jobP(job));
}, { multipart: true });
post(`${A}/imports/stripe-mapping`, (ctx) => {
  const { fields, file } = ctx.form;
  const kind = fields.kind || 'customers';
  const f = {};
  if (!file || file.fieldname !== 'file') f.file = 'is required';
  if (!['customers', 'products', 'subscriptions'].includes(kind)) f.kind = 'must be customers, products or subscriptions';
  fail(f);
  const rows = file.data ? Math.max(1, file.data.toString('utf8').split(/\r?\n/).filter(Boolean).length - 1) : 40;
  const job = startJob(ctx, {
    kind: 'stripe_mapping', stages: [['parse_csv', rows], [`map_${kind}`, rows]], duration: 6_000, options: { kind, filename: file.filename, rows },
    hook: (j) => {
      const n = applyMapping(kind, rows);
      jobLog(j, 'info', `Applied ${n} Stripe ${kind} mapping(s)`);
    },
  });
  return created(jobP(job));
}, { multipart: true });
post(`${A}/imports/stripe-auto-match`, (ctx) => {
  const b = ctx.body;
  const kinds = ['customers', 'products', 'subscriptions'].filter((k) => bool(b[k], false));
  if (!kinds.length) throw E.invalid({ customers: 'select at least one resource to match' });
  const stages = kinds.map((k) => [k, Math.max(1, unmapped[k]().length)]);
  const job = startJob(ctx, {
    kind: 'stripe_auto_match', stages, duration: 8_000, options: { customers: kinds.includes('customers'), products: kinds.includes('products'), subscriptions: kinds.includes('subscriptions') },
    hook: (j) => {
      for (const k of kinds) {
        const total = unmapped[k]().length;
        const matched = applyMapping(k, Math.ceil(total * 0.8));
        jobLog(j, 'info', `Matched ${matched} of ${total} ${k}`);
        const left = unmapped[k]().length;
        if (left) {
          j.progress.warnings += left;
          jobLog(j, 'warn', `${left} ${k} could not be matched automatically (see the mapping report)`);
        }
      }
    },
  });
  return created(jobP(job));
});
get(`${A}/imports/mapping-report.csv`, () => {
  const rows = [
    ...unmapped.customers().map((c) => ['customer', c.id, c.name, c.email, 'no stripe_customer_id']),
    ...unmapped.products().map((v) => ['variant', v.id, `${productOf(v)?.name} / ${v.name}`, '', 'no stripe_product_id']),
    ...unmapped.subscriptions().map((s) => ['subscription', s.id, `${s.product_name} / ${s.variant_name}`, s.user_email, 'no stripe_subscription_id']),
  ];
  return attachment(`stripe-mapping-report-${isoDate(now())}.csv`, toCsv(['kind', 'id', 'name', 'email', 'problem'], rows), 'text/csv; charset=utf-8');
});
get(`${A}/imports`, (ctx) => json(list(ctx, db.imports.map(jobP), { search: ['kind', 'status', 'created_by'], filters: { status: 'status', kind: 'kind' } })));
get(`${A}/imports/:id`, (ctx) => json(jobP(must('imports', ctx.params.id, 'import job'))));
get(`${A}/imports/:id/logs`, (ctx) => {
  const job = must('imports', ctx.params.id, 'import job');
  const after = parseInt(ctx.query.after, 10) || 0;
  return json(db.importLogs.filter((l) => l.job_id === job.id && l.id > after).map(({ id, level, message, created_at: at }) => ({ id, level, message, created_at: at })));
});
post(`${A}/imports/:id/cancel`, (ctx) => {
  const job = must('imports', ctx.params.id, 'import job');
  if (!['pending', 'running'].includes(job.status)) throw E.conflict(`Import is already ${job.status}`);
  job.status = 'cancelled';
  job.finished_at = iso(now());
  jobLog(job, 'warn', 'Import cancelled by user');
  return json(jobP(job));
});

// ===============================================================================================
// Storefront (customer-facing) API
// ===============================================================================================
const customerEmail = (ctx) => customerSessions.get(ctx.cookies.df_customer) || null;
function requireCustomer(ctx) {
  const email = customerEmail(ctx);
  if (!email) throw E.unauth('Please sign in to view your orders');
  return email;
}
const customerOrders = (email) => db.orders.filter((o) => o.user_email.toLowerCase() === email.toLowerCase() && !o.test_mode && o.status !== 'failed').sort((a, b) => b.created_at.localeCompare(a.created_at));
const signedExpires = () => Math.floor((now() + 3600_000) / 1000);
const billingUrl = () => `/billing?expires=${signedExpires()}&signature=${randomHex(32)}`;

post(`${S}/auth/magic-link`, (ctx) => {
  if (isEmail(ctx.body.email)) console.log(`  [magic-link] ${ctx.body.email} -> ${PUBLIC_URL}/my-orders/verify?token=demo`);
  return noContent();
}, PUBLIC);
post(`${S}/auth/verify`, (ctx) => {
  const token = String(ctx.body.token || '');
  if (!token || token === 'invalid' || token === 'expired') throw new ApiError(400, 'invalid_token', 'This sign-in link is invalid or has expired.');
  const sid = randomHex(40);
  customerSessions.set(sid, CUSTOMER_EMAIL);
  return json({ email: CUSTOMER_EMAIL }, 200, { 'set-cookie': `df_customer=${sid}; Path=/; HttpOnly; SameSite=Lax; Max-Age=2592000` });
}, PUBLIC);
post(`${S}/auth/logout`, (ctx) => {
  customerSessions.delete(ctx.cookies.df_customer);
  return new Out(204, '', { 'set-cookie': 'df_customer=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0' });
}, PUBLIC);
get(`${S}/me/orders`, (ctx) => {
  const email = requireCustomer(ctx);
  const orders = customerOrders(email).map((o) => {
    const items = db.orderItems.filter((i) => i.order_id === o.id);
    return {
      identifier: o.identifier, order_number: o.order_number, created_at: o.created_at, status: o.status, total_formatted: money(o.total, o.currency), currency: o.currency,
      items: items.map((i) => ({ product_name: i.product_name, variant_name: i.variant_name })), license_key_count: db.licenseKeys.filter((k) => k.order_id === o.id).length, file_count: orderFiles(o).length,
    };
  });
  const orderIds = new Set(customerOrders(email).map((o) => o.id));
  const subscriptions = db.subscriptions.filter((s) => orderIds.has(s.order_id)).map((s) => ({
    id: s.id, status: s.status, status_formatted: subscriptionP(s).status_formatted, product_name: s.product_name, variant_name: s.variant_name, renews_at: s.renews_at, ends_at: s.ends_at, billing_url: billingUrl(),
  }));
  return json({ email, orders, subscriptions });
}, PUBLIC);
get(`${S}/orders/:identifier`, (ctx) => {
  const o = db.orders.find((x) => x.identifier === ctx.params.identifier);
  if (!ctx.query.signature) {
    const email = requireCustomer(ctx);
    if (!o || o.user_email.toLowerCase() !== email.toLowerCase()) throw o ? E.forbidden('This order belongs to a different account') : E.notFound('order');
  } else if (!o) {
    throw E.notFound('order');
  }
  const items = db.orderItems.filter((i) => i.order_id === o.id);
  const variants = items.map((i) => variantOf(i.variant_id)).filter(Boolean);
  const links = [];
  for (const v of variants) for (const l of v.links || []) if (!links.some((x) => x.url === l.url)) links.push(l);
  const keysAllowed = ['paid', 'partial_refund'].includes(o.status);
  const subs = db.subscriptions.filter((s) => s.order_id === o.id);
  const m = (c) => money(c, o.currency);
  return json({
    identifier: o.identifier, order_number: o.order_number, status: o.status, status_formatted: capitalize(o.status), created_at: o.created_at, user_name: o.user_name, user_email: o.user_email,
    currency: o.currency, subtotal_formatted: m(o.subtotal), discount_total_formatted: m(o.discount_total), tax_formatted: m(o.tax), total_formatted: m(o.total), refunded: o.refunded,
    card_brand: o.card_brand, card_last_four: o.card_last_four,
    items: items.map((i) => ({ product_name: i.product_name, variant_name: i.variant_name, quantity: i.quantity, price_formatted: m(i.price) })),
    license_keys: keysAllowed ? db.licenseKeys.filter((k) => k.order_id === o.id).map(licenseKeyP).map((k) => ({
      key: k.key, status: k.status, status_formatted: k.status_formatted, activation_limit: k.activation_limit, instances_count: k.instances_count, expires_at: k.expires_at, product_name: k.product_name, variant_name: k.variant_name,
    })) : [],
    files: orderFiles(o).map((f) => ({
      id: f.id, name: f.name, extension: f.extension, size_formatted: humanSize(f.size), version: f.version, product_name: productOf(variantOf(f.variant_id))?.name || '', variant_name: variantOf(f.variant_id)?.name || '',
      download_url: `/download/${f.identifier}?expires=${signedExpires()}&signature=${randomHex(32)}`,
    })),
    links: keysAllowed ? links : [],
    subscriptions: subs.map((s) => ({ id: s.id, status: s.status, status_formatted: subscriptionP(s).status_formatted, product_name: s.product_name, renews_at: s.renews_at, ends_at: s.ends_at })),
    billing_url: subs.length || o.stripe_customer_id ? billingUrl() : null,
    invoice_url: PAIDISH.includes(o.status) ? `https://pay.stripe.com/invoice/mock_${o.identifier.slice(0, 12)}` : null,
  });
}, PUBLIC);
get(`${S}/billing-url`, (ctx) => {
  requireCustomer(ctx);
  return json({ url: BILLING_PORTAL });
}, PUBLIC);

// ------------------------------------------------------------------------------------------ misc public routes
get('/download/:id', (ctx) => {
  const f = db.files.find((x) => x.identifier === ctx.params.id);
  if (f) f.downloads_count += 1;
  return attachment(f ? `${f.name}.${f.extension}` : 'download.txt', `Mock download for ${f ? f.name : ctx.params.id}\n`);
}, PUBLIC);
get('/billing', () => redirect(BILLING_PORTAL), PUBLIC);
get('/healthz', () => new Out(200, 'ok', { 'content-type': 'text/plain' }), PUBLIC);

// ===============================================================================================
// Dispatcher
// ===============================================================================================
function parseCookies(header = '') {
  const out = {};
  for (const part of header.split(';')) {
    const i = part.indexOf('=');
    if (i > 0) out[part.slice(0, i).trim()] = decodeURIComponent(part.slice(i + 1).trim());
  }
  return out;
}

async function dispatch(req, url) {
  const method = req.method;
  const path = url.pathname.replace(/\/+$/, '') || '/';
  const isApi = path.startsWith(`${A}/`) || path.startsWith(`${S}/`);
  let matched = null;
  let pathMatched = false;
  for (const r of routes) {
    const m = r.re.exec(path);
    if (!m) continue;
    pathMatched = true;
    if (r.method !== method) continue;
    matched = { r, params: Object.fromEntries(r.keys.map((k, i) => [k, decodeURIComponent(m[i + 1])])) };
    break;
  }
  if (!matched) {
    if (pathMatched) throw new ApiError(405, 'method_not_allowed', `${method} is not allowed on ${path}`);
    throw E.notFound(`route ${method} ${path}`);
  }
  const { r, params } = matched;
  const cookies = parseCookies(req.headers.cookie);
  const ctx = {
    req, url, method, path, params, cookies, query: Object.fromEntries(url.searchParams), body: {}, form: null, sid: cookies.df_admin, user: null,
  };
  const uid = sessions.get(cookies.df_admin);
  ctx.user = uid ? db.users.find((u) => u.id === uid && !u.disabled) || null : null;
  if (isApi && WRITE.has(method) && req.headers['x-requested-with'] !== 'df-admin') throw E.forbidden('Missing X-Requested-With: df-admin header');
  if (path.startsWith(A) && !r.opts.public) {
    if (!ctx.user) throw E.unauth();
    const need = WRITE.has(method) ? r.opts.write || 'admin' : r.opts.read || 'viewer';
    if (RANK[ctx.user.role] < RANK[need]) throw E.forbidden(ctx.user.role === 'viewer' && WRITE.has(method) ? 'Viewers have read-only access' : `Requires the ${need} role`);
  }
  if (WRITE.has(method)) {
    if (r.opts.multipart) ctx.form = await readMultipart(req);
    else ctx.body = await readJson(req);
  }
  const result = await r.handler(ctx);
  const out = result instanceof Out ? result : result === undefined ? noContent() : json(result);
  if (WRITE.has(method) && ctx.user && path.startsWith(A) && !path.includes('/auth/') && out.status < 300) recordAudit(ctx, path);
  return out;
}

function recordAudit(ctx, path) {
  const seg = path.slice(A.length + 1).split('/');
  const verb = seg.length > 2 && !/^\d+$/.test(seg[2]) ? seg[2] : { POST: 'create', PATCH: 'update', PUT: 'update', DELETE: 'delete' }[ctx.method];
  const id = /^\d+$/.test(seg[1] || '') ? Number(seg[1]) : null;
  if (verb === 'refund' || (seg[0] === 'settings' && ctx.method === 'PUT')) return; // audited explicitly in the handler
  audit(ctx, `${seg[0].replace(/s$/, '').replace(/-/g, '_')}.${verb}`, seg[0], id, ctx.method === 'DELETE' ? {} : { fields: Object.keys(ctx.body || {}).filter((k) => !/password|secret|token|key$/.test(k)) });
}

const server = http.createServer(async (req, res) => {
  const t0 = Date.now();
  const url = new URL(req.url, 'http://mock.local');
  let out;
  try {
    if (LATENCY && url.pathname !== '/healthz') await sleep(30 + Math.random() * 120);
    if (req.method === 'OPTIONS') out = noContent();
    else out = await dispatch(req, url);
  } catch (err) {
    if (err instanceof ApiError) {
      out = json({ error: { code: err.code, message: err.message, ...(err.fields ? { fields: err.fields } : {}) } }, err.status);
    } else {
      console.error(err);
      out = json({ error: { code: 'internal', message: 'Internal mock server error: ' + (err?.message || err) } }, 500);
    }
  }
  req.resume();
  const headers = { 'cache-control': 'no-store', ...out.headers };
  res.writeHead(out.status, headers);
  res.end(out.status === 204 || req.method === 'HEAD' ? undefined : out.body);
  console.log(`${new Date().toISOString().slice(11, 19)} ${req.method.padEnd(6)} ${url.pathname}${url.search} -> ${out.status} (${Date.now() - t0}ms)`);
});

server.listen(PORT, () => {
  console.log(`\nMock backend listening on http://localhost:${PORT}`);
  console.log('  Admin demo users (password "' + DEMO_PASSWORD + '"): owner@example.com | admin@example.com | viewer@example.com');
  console.log(`  Stores: ${db.stores.length}   setup_required: ${setupRequired}${setupRequired ? `   setup_token: ${SETUP_TOKEN}` : ''}   stripe_login: ${STRIPE_LOGIN}`);
  console.log(`  Storefront: any magic-link token signs in as ${CUSTOMER_EMAIL} (tokens "invalid" / "expired" are rejected)`);
  console.log('  Import tip: LS API key "fail" makes the import fail midway; any other non-empty key succeeds.\n');
});
