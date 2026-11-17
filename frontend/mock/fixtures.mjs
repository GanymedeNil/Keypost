// Deterministic in-memory fixtures for the mock backend. Dates are relative to "now" so charts always have data.
import { DAY, HOUR, iso, makeRng } from './lib.mjs';

const PUBLIC_URL = process.env.PUBLIC_URL || 'http://localhost:3000';

export const WEBHOOK_EVENTS = [
  'order_created',
  'order_refunded',
  'subscription_created',
  'subscription_updated',
  'subscription_cancelled',
  'subscription_resumed',
  'subscription_expired',
  'subscription_paused',
  'subscription_unpaused',
  'subscription_payment_success',
  'subscription_payment_failed',
  'subscription_payment_recovered',
  'subscription_payment_refunded',
  'license_key_created',
  'license_key_updated',
  'affiliate_activated',
];

export const STRIPE_REQUIRED_EVENTS = [
  'checkout.session.completed',
  'checkout.session.async_payment_succeeded',
  'invoice.paid',
  'invoice.payment_failed',
  'customer.subscription.created',
  'customer.subscription.updated',
  'customer.subscription.deleted',
  'charge.refunded',
  'charge.dispute.created',
];

const FIRST = ['Alex', 'Jordan', 'Taylor', 'Morgan', 'Casey', 'Riley', 'Sam', 'Jamie', 'Avery', 'Quinn', 'Drew', 'Robin', 'Charlie', 'Skyler', 'Emery', 'Hayden', 'Kai', 'Noa', 'Lena', 'Mateo', 'Sofia', 'Liam', 'Yuki', 'Wei', 'Priya', 'Omar', 'Elena', 'Hugo', 'Nina', 'Tomas'];
const LAST = ['Smith', 'Johnson', 'Lee', 'Garcia', 'Martin', 'Nguyen', 'Brown', 'Tanaka', 'Schmidt', 'Rossi', 'Dubois', 'Patel', 'Kim', 'Silva', 'Novak', 'Andersson', 'Cohen', 'Murphy', 'Chen', 'Haddad'];
const PLACES = [
  { country: 'US', city: 'Austin', region: 'TX', zip: '78701' },
  { country: 'US', city: 'Brooklyn', region: 'NY', zip: '11201' },
  { country: 'GB', city: 'London', region: 'England', zip: 'E1 6AN' },
  { country: 'DE', city: 'Berlin', region: 'BE', zip: '10115' },
  { country: 'FR', city: 'Lyon', region: 'ARA', zip: '69001' },
  { country: 'CA', city: 'Toronto', region: 'ON', zip: 'M5V 2T6' },
  { country: 'AU', city: 'Melbourne', region: 'VIC', zip: '3000' },
  { country: 'JP', city: 'Osaka', region: 'Osaka', zip: '530-0001' },
  { country: 'CN', city: 'Shenzhen', region: 'GD', zip: '518000' },
  { country: 'BR', city: 'Sao Paulo', region: 'SP', zip: '01000-000' },
  { country: 'IN', city: 'Pune', region: 'MH', zip: '411001' },
  { country: 'NL', city: 'Utrecht', region: 'UT', zip: '3511' },
];
const DEVICES = ['MacBook-Pro-14', 'Studio-PC', 'ubuntu-workstation', 'Mac-mini-office', 'Surface-Laptop', 'build-agent-02', 'design-imac'];

export function buildDb({ storeCount = 2, now = Date.now() } = {}) {
  const r = makeRng(20261117);
  const db = {
    now,
    seq: {},
    stores: [],
    products: [],
    variants: [],
    prices: [],
    files: [],
    customers: [],
    orders: [],
    orderItems: [],
    licenseKeys: [],
    instances: [],
    subscriptions: [],
    invoices: [],
    webhooks: [],
    deliveries: [],
    stripeEvents: [],
    emailLogs: [],
    auditLogs: [],
    apiKeys: [],
    users: [],
    passwords: new Map(),
    imports: [],
    importLogs: [],
    catalog: [],
    settings: null,
  };
  const nextId = (name) => {
    db.seq[name] = (db.seq[name] || 0) + 1;
    return db.seq[name];
  };
  db.nextId = nextId;
  const add = (coll, obj) => {
    const rec = { id: nextId(coll), ...obj };
    db[coll].push(rec);
    return rec;
  };

  // ------------------------------------------------------------------ users
  const mkUser = (email, name, role, extra = {}) => {
    const u = add('users', {
      email, name, role, stripe_account_id: null, stripe_account_label: null, disabled: false, has_password: true,
      last_login_at: iso(now - r.int(1, 40) * HOUR), created_at: iso(now - 120 * DAY), ...extra,
    });
    db.passwords.set(u.id, 'password1234');
    return u;
  };
  mkUser('owner@example.com', 'Olivia Owner', 'owner', { stripe_account_id: 'acct_1MockOwner01', stripe_account_label: 'Acme Software (acct_1MockOwner01)' });
  mkUser('admin@example.com', 'Adam Admin', 'admin');
  mkUser('viewer@example.com', 'Vera Viewer', 'viewer');

  // ------------------------------------------------------------------ stores
  const storeDefs = [
    { name: 'Acme Software', slug: 'acme-software', domain: 'acme-software.example.com', currency: 'USD', country: 'US' },
    { name: 'Acme Studio', slug: 'acme-studio', domain: 'studio.example.com', currency: 'USD', country: 'DE' },
  ].slice(0, Math.max(1, storeCount));
  storeDefs.forEach((s, i) => {
    add('stores', {
      name: s.name, slug: s.slug, domain: s.domain, url: `https://${s.domain}`, avatar_url: '', plan: 'fresh', country: s.country, currency: s.currency,
      support_email: `support@${s.domain.split('.').slice(-2).join('.')}`, email_from_name: s.name,
      receipt_subject: 'Your {{store}} order receipt #{{order_number}}',
      receipt_intro: 'Thanks for your purchase! Your license keys and downloads are available below.',
      stripe_account_id: i === 0 ? 'acct_1MockOwner01' : null,
      total_sales: 0, total_revenue: 0, thirty_day_sales: 0, thirty_day_revenue: 0,
      created_at: iso(now - (200 - i * 30) * DAY), updated_at: iso(now - 10 * DAY),
    });
  });
  const storeIds = db.stores.map((s) => s.id);
  const storeOf = (i) => storeIds[Math.min(i, storeIds.length - 1)];

  // ------------------------------------------------------------------ products / variants / prices / files
  const mkProduct = (store, name, description, status, test = false, extra = {}) =>
    add('products', {
      store_id: storeOf(store), name, slug: name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''), description,
      status, thumb_url: '', large_thumb_url: '', stripe_product_id: null, test_mode: test,
      created_at: iso(now - r.int(60, 160) * DAY), updated_at: iso(now - r.int(1, 30) * DAY), ...extra,
    });
  const mkVariant = (product, name, o = {}) =>
    add('variants', {
      product_id: product.id, name, slug: name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''), description: o.description || '',
      links: o.links || [], sort: o.sort ?? 0, status: o.status || 'published',
      has_license_keys: !!o.license, license_activation_limit: o.license ? (o.limit ?? 1) : 0, is_license_limit_unlimited: !!(o.license && o.limit === null),
      license_length_value: o.length ? o.length[0] : 1, license_length_unit: o.length ? o.length[1] : 'years', is_license_length_unlimited: o.license ? !o.length : false,
      stripe_product_id: o.stripe_product_id || null, test_mode: product.test_mode,
      created_at: product.created_at, updated_at: iso(now - r.int(1, 20) * DAY),
    });
  const mkPrice = (variant, o) =>
    add('prices', {
      variant_id: variant.id, category: o.category || 'one_time', scheme: o.scheme || 'standard', unit_price: o.unit ?? 0, unit_price_decimal: null,
      setup_fee_enabled: !!o.setup, setup_fee: o.setup || 0, package_size: o.package_size ?? 1, tiers: o.tiers || [],
      renewal_interval_unit: o.interval ? o.interval[0] : null, renewal_interval_quantity: o.interval ? o.interval[1] : null,
      trial_interval_unit: o.trial ? o.trial[0] : null, trial_interval_quantity: o.trial ? o.trial[1] : null,
      min_price: o.min ?? 0, suggested_price: o.suggested ?? 0, tax_code: o.category === 'one_time' || !o.category ? 'eservice' : 'saas', currency: 'USD',
      stripe_price_id: o.stripe_price_id || null, created_at: variant.created_at, updated_at: variant.updated_at,
    });
  const mkFile = (variant, name, ext, bytes, version, o = {}) =>
    add('files', {
      variant_id: variant.id, identifier: r.uuid(), name, extension: ext, size: bytes, version, sort: o.sort ?? 0, status: o.status || 'published',
      storage_driver: 'local', content_type: ext === 'zip' ? 'application/zip' : ext === 'pdf' ? 'application/pdf' : 'application/octet-stream',
      sha256: r.hex(64), downloads_count: o.downloads ?? r.int(5, 400), test_mode: variant.test_mode,
      created_at: iso(now - r.int(10, 90) * DAY), updated_at: iso(now - r.int(1, 9) * DAY),
    });

  const P1 = mkProduct(0, 'Pixel Studio', 'Professional pixel-art editor for macOS and Windows.', 'published', false, { stripe_product_id: 'prod_MockPixel01' });
  const V1 = mkVariant(P1, 'Personal', { license: true, limit: 2, links: [{ title: 'Documentation', url: 'https://docs.example.com/pixel-studio' }, { title: 'Release notes', url: 'https://example.com/pixel-studio/changelog' }], sort: 1, stripe_product_id: 'prod_MockPixel01' });
  const V2 = mkVariant(P1, 'Team (5 seats)', { license: true, limit: 5, sort: 2 });
  mkPrice(V1, { unit: 4900, stripe_price_id: 'price_MockPixelPersonal' });
  mkPrice(V2, { unit: 19900 });
  mkFile(V1, 'Pixel Studio for macOS', 'dmg', 93_452_288, '2.4.1');
  mkFile(V1, 'Pixel Studio for Windows', 'exe', 81_234_944, '2.4.1', { sort: 1 });
  mkFile(V2, 'Pixel Studio Team Installer', 'zip', 160_432_128, '2.4.1');

  const P2 = mkProduct(0, 'Notekit', 'Fast, local-first notes with sync. Billed monthly or yearly.', 'published', false, { stripe_product_id: 'prod_MockNotekit01' });
  const V3 = mkVariant(P2, 'Monthly', { license: true, limit: 3, length: [1, 'months'], sort: 1, stripe_product_id: 'prod_MockNotekit01', links: [{ title: 'Getting started', url: 'https://docs.example.com/notekit' }] });
  const V4 = mkVariant(P2, 'Yearly', { license: true, limit: 3, length: [1, 'years'], sort: 2 });
  mkPrice(V3, { category: 'subscription', unit: 900, interval: ['month', 1], trial: ['day', 14], stripe_price_id: 'price_MockNotekitMonthly' });
  mkPrice(V4, { category: 'subscription', unit: 9000, interval: ['year', 1], trial: ['day', 14] });
  mkFile(V3, 'Notekit Desktop', 'zip', 54_525_952, '1.9.0');

  const P3 = mkProduct(0, 'Line UI Icon Pack', '1,800 hand-drawn line icons in SVG, PNG and Figma.', 'published', false, { stripe_product_id: 'prod_MockIcons01' });
  const V5 = mkVariant(P3, 'Standard', { sort: 1, stripe_product_id: 'prod_MockIcons01' });
  const V6 = mkVariant(P3, 'Extended', { sort: 2 });
  mkPrice(V5, { unit: 2900, stripe_price_id: 'price_MockIconsStandard' });
  mkPrice(V6, { unit: 7900 });
  mkFile(V5, 'line-icons-standard', 'zip', 24_117_248, '3.2.0');
  mkFile(V6, 'line-icons-extended', 'zip', 71_303_168, '3.2.0');

  const P4 = mkProduct(0, 'Ship Faster (Course)', 'A 12-hour video course on shipping indie products.', 'published');
  const V7 = mkVariant(P4, 'Course', { sort: 1, links: [{ title: 'Private community', url: 'https://community.example.com/join' }] });
  const V8 = mkVariant(P4, 'Course + Coaching', { sort: 2, links: [{ title: 'Book your coaching call', url: 'https://cal.example.com/ship-faster' }] });
  mkPrice(V7, { unit: 14900 });
  mkPrice(V8, { unit: 34900 });
  mkFile(V7, 'ship-faster-workbook', 'pdf', 4_718_592, '2024.2');

  const P5 = mkProduct(1, 'Soundscapes Vol. 1', 'Ambient sample pack: 120 loops, pay what you want.', 'published');
  const V9 = mkVariant(P5, 'Full pack', { sort: 1 });
  mkPrice(V9, { category: 'pwyw', unit: 1500, min: 500, suggested: 1500 });
  mkFile(V9, 'soundscapes-vol1-wav', 'zip', 412_000_000, '1.0.0');
  mkFile(V9, 'soundscapes-vol1-mp3', 'zip', 98_500_000, '1.0.0', { sort: 1 });

  const P6 = mkProduct(1, 'Lumen Typeface', 'A friendly geometric sans with 9 weights.', 'published', false, { stripe_product_id: 'prod_MockLumen01' });
  const V10 = mkVariant(P6, 'Desktop License', { license: true, limit: 3, sort: 1, stripe_product_id: 'prod_MockLumen01' });
  const V11 = mkVariant(P6, 'Web License (yearly)', { license: true, limit: null, length: [1, 'years'], sort: 2 });
  mkPrice(V10, { unit: 5900, stripe_price_id: 'price_MockLumenDesktop' });
  mkPrice(V11, {
    category: 'subscription', scheme: 'graduated', unit: 12000, interval: ['year', 1],
    tiers: [{ last_unit: 5, unit_price: 12000, unit_price_decimal: null, fixed_fee: 0 }, { last_unit: 'inf', unit_price: 9000, unit_price_decimal: null, fixed_fee: 0 }],
  });
  mkFile(V10, 'Lumen Desktop Fonts', 'zip', 8_912_896, '1.3.0');
  mkFile(V11, 'Lumen Web Fonts (woff2)', 'zip', 2_097_152, '1.3.0');

  const P7 = mkProduct(1, 'Cheatsheet Bundle', 'Free design cheatsheets (lead magnet).', 'draft');
  const V12 = mkVariant(P7, 'Free sample', { status: 'draft', sort: 1 });
  mkPrice(V12, { category: 'lead_magnet', unit: 0 });
  mkFile(V12, 'design-cheatsheets', 'pdf', 1_258_291, '1.0', { status: 'draft', downloads: 0 });

  const PT = mkProduct(0, 'Sandbox App (test)', 'Used for testing the full checkout flow.', 'published', true);
  const V13 = mkVariant(PT, 'Test License', { license: true, limit: 2, sort: 1 });
  const V14 = mkVariant(PT, 'Test Subscription', { license: true, limit: 1, length: [1, 'months'], sort: 2 });
  mkPrice(V13, { unit: 1000 });
  mkPrice(V14, { category: 'subscription', unit: 500, interval: ['month', 1] });
  mkFile(V13, 'sandbox-app', 'zip', 3_145_728, '0.1.0');
  mkFile(V14, 'sandbox-app-pro', 'zip', 5_242_880, '0.1.0');

  const variantById = (id) => db.variants.find((v) => v.id === id);
  const productOfVariant = (v) => db.products.find((p) => p.id === v.product_id);
  const priceOfVariant = (v) => db.prices.find((p) => p.variant_id === v.id);

  // ------------------------------------------------------------------ customers
  const emailsUsed = new Set();
  const mkCustomer = (name, email, test, place, ageDays, storeIdx = 0) =>
    add('customers', {
      store_id: storeOf(storeIdx), name, email, status: 'subscribed', city: place.city, region: place.region, country: place.country,
      stripe_customer_id: null, test_mode: test, created_at: iso(now - ageDays * DAY), updated_at: iso(now - Math.max(0, ageDays - 5) * DAY),
      _place: place,
    });
  const demo = mkCustomer('Casey Customer', 'customer@example.com', false, PLACES[0], 75, 0);
  emailsUsed.add(demo.email);
  for (let i = 0; i < 44; i++) {
    const first = r.pick(FIRST);
    const last = r.pick(LAST);
    let email = `${first}.${last}@${r.pick(['gmail.com', 'outlook.com', 'proton.me', 'fastmail.com', 'example.org', 'icloud.com'])}`.toLowerCase();
    while (emailsUsed.has(email)) email = email.replace('@', `${r.int(2, 99)}@`);
    emailsUsed.add(email);
    mkCustomer(`${first} ${last}`, email, i >= 38, r.pick(PLACES), r.int(2, 85), i % 3 === 2 ? 1 : 0);
  }
  db.customers.forEach((c, i) => {
    // ~20% have no Stripe customer id (e.g. migrated from Lemon Squeezy)
    c.stripe_customer_id = i === 0 || i % 5 !== 3 ? `cus_Mock${c.test_mode ? 'T' : 'L'}${r.hex(10)}` : null;
  });

  // ------------------------------------------------------------------ orders (+items, keys, subs, invoices)
  const cards = ['visa', 'mastercard', 'amex', 'discover'];
  const SUB_STATUS_CYCLE = ['active', 'active', 'on_trial', 'active', 'past_due', 'cancelled', 'paused', 'active', 'unpaid', 'expired', 'active', 'on_trial'];
  let subCycle = 0;
  const mkKey = () => r.uuid();
  const instNames = () => r.pick(DEVICES);

  function createOrder({ customer, variants, status, createdAt, source, test, forceSubStatus }) {
    const lines = variants.map((v) => {
      const price = priceOfVariant(v);
      let unit = price.unit_price;
      if (price.category === 'pwyw') unit = r.int(price.min_price / 100, 3000 / 100) * 100;
      return { v, price, unit, qty: 1 };
    });
    const subtotal = lines.reduce((s, l) => s + l.unit * l.qty, 0);
    const discount = subtotal > 0 && r.chance(0.12) ? Math.round(subtotal * 0.1) : 0;
    const taxRate = customer._place.country === 'US' ? 0 : r.pick([0, 19, 20, 8]);
    const tax = Math.round((subtotal - discount) * (taxRate / 100));
    const total = subtotal - discount + tax;
    let refundedAmount = 0;
    if (status === 'refunded') refundedAmount = total;
    if (status === 'partial_refund') refundedAmount = Math.max(1, Math.round(total * 0.4));
    const stripeBacked = source === 'stripe';
    const hasSub = lines.some((l) => l.price.category === 'subscription');
    const order = add('orders', {
      store_id: customer.store_id, customer_id: customer.id, identifier: r.uuid(), order_number: 0,
      user_name: customer.name, user_email: customer.email, currency: 'USD', currency_rate: 1,
      subtotal, setup_fee: 0, discount_total: discount, tax, total, refunded_amount: refundedAmount,
      subtotal_usd: subtotal, discount_total_usd: discount, tax_usd: tax, total_usd: total, refunded_amount_usd: refundedAmount,
      tax_name: taxRate ? 'VAT' : 'Sales Tax', tax_rate: taxRate, tax_inclusive: false, status,
      refunded: refundedAmount > 0, refunded_at: refundedAmount > 0 ? iso(createdAt + r.int(1, 6) * DAY) : null,
      affiliate_id: null, referral_amount: 0, country: customer.country, city: customer.city, postal_code: customer._place.zip,
      card_brand: status === 'failed' || total === 0 ? null : r.pick(cards), card_last_four: status === 'failed' || total === 0 ? null : String(r.int(1000, 9999)),
      custom_data: r.chance(0.2) ? { campaign: r.pick(['launch', 'blackfriday', 'newsletter']) } : {},
      stripe_checkout_session: stripeBacked ? `cs_${test ? 'test' : 'live'}_${r.hex(24)}` : null,
      stripe_payment_intent: stripeBacked && status !== 'pending' ? `pi_${r.hex(24)}` : null,
      stripe_invoice_id: stripeBacked && hasSub ? `in_${r.hex(24)}` : null,
      stripe_customer_id: stripeBacked ? customer.stripe_customer_id : null,
      source, test_mode: test, created_at: iso(createdAt), updated_at: iso(createdAt + r.int(1, 5) * 60_000),
    });
    const paidish = ['paid', 'refunded', 'partial_refund'].includes(status);
    for (const l of lines) {
      const prod = productOfVariant(l.v);
      const item = add('orderItems', {
        order_id: order.id, product_id: prod.id, variant_id: l.v.id, price_id: l.price.id, product_name: prod.name, variant_name: l.v.name, price: l.unit, quantity: l.qty,
      });
      if (!paidish) continue;
      let sub = null;
      if (l.price.category === 'subscription') {
        const st = forceSubStatus || SUB_STATUS_CYCLE[subCycle++ % SUB_STATUS_CYCLE.length];
        const unit = l.price.renewal_interval_unit;
        const periodMs = (unit === 'year' ? 365 : unit === 'week' ? 7 : unit === 'day' ? 1 : 30) * DAY;
        const trialEnds = l.price.trial_interval_quantity ? createdAt + l.price.trial_interval_quantity * DAY : null;
        let renews = createdAt + periodMs;
        while (renews < now && st !== 'expired' && st !== 'cancelled') renews += periodMs;
        const cancelled = st === 'cancelled' || st === 'expired';
        const endsAt = st === 'expired' ? now - r.int(2, 20) * DAY : st === 'cancelled' ? now + r.int(3, 25) * DAY : null;
        if (st === 'on_trial') renews = now + r.int(2, 12) * DAY;
        sub = add('subscriptions', {
          store_id: order.store_id, customer_id: customer.id, order_id: order.id, order_item_id: item.id, product_id: prod.id, variant_id: l.v.id, price_id: l.price.id,
          product_name: prod.name, variant_name: l.v.name, user_name: customer.name, user_email: customer.email, status: st, status_formatted: st === 'on_trial' ? 'On trial' : st.charAt(0).toUpperCase() + st.slice(1).replace('_', ' '),
          card_brand: order.card_brand || 'visa', card_last_four: order.card_last_four || '4242', payment_processor: 'stripe',
          pause: st === 'paused' ? { mode: 'void', resumes_at: iso(now + 12 * DAY) } : null, cancelled,
          trial_ends_at: trialEnds ? iso(trialEnds) : null, billing_anchor: new Date(createdAt).getUTCDate(), quantity: 1,
          renews_at: st === 'expired' || st === 'cancelled' ? null : iso(renews), ends_at: endsAt ? iso(endsAt) : null,
          stripe_subscription_id: source === 'stripe' ? `sub_${r.hex(14)}` : null, stripe_customer_id: customer.stripe_customer_id,
          test_mode: test, created_at: iso(createdAt), updated_at: iso(Math.min(now, createdAt + r.int(1, 25) * DAY)),
        });
        // invoices
        const n = Math.max(1, Math.min(5, Math.floor((now - createdAt) / periodMs) + 1));
        for (let k = 0; k < n; k++) {
          const at = createdAt + k * periodMs;
          if (at > now) break;
          const invStatus = k === n - 1 && st === 'past_due' ? 'pending' : k === n - 1 && st === 'unpaid' ? 'void' : status === 'refunded' && k === 0 ? 'refunded' : 'paid';
          add('invoices', {
            subscription_id: sub.id, customer_id: customer.id, user_name: customer.name, user_email: customer.email, billing_reason: k === 0 ? 'initial' : 'renewal',
            card_brand: sub.card_brand, card_last_four: sub.card_last_four, currency: 'USD', status: invStatus, refunded: invStatus === 'refunded', refunded_at: invStatus === 'refunded' ? iso(at + DAY) : null,
            subtotal: l.unit, discount_total: 0, tax: 0, total: l.unit, refunded_amount: invStatus === 'refunded' ? l.unit : 0,
            invoice_url: `https://pay.stripe.com/invoice/mock_${r.hex(12)}`, stripe_invoice_id: source === 'stripe' ? `in_${r.hex(24)}` : null, created_at: iso(at),
          });
        }
      }
      if (l.v.has_license_keys) {
        const keyStr = mkKey();
        let expires = null;
        if (sub) expires = sub.status === 'expired' ? sub.ends_at : sub.renews_at || sub.ends_at;
        else if (!l.v.is_license_length_unlimited) expires = iso(createdAt + 365 * DAY);
        const limit = l.v.is_license_limit_unlimited ? null : l.v.license_activation_limit;
        const disabled = status === 'refunded' || (status === 'paid' && r.chance(0.04));
        const key = add('licenseKeys', {
          store_id: order.store_id, customer_id: customer.id, order_id: order.id, order_item_id: item.id, product_id: prod.id, variant_id: l.v.id, subscription_id: sub ? sub.id : null,
          product_name: prod.name, variant_name: l.v.name, user_name: customer.name, user_email: customer.email, key: keyStr, key_short: `XXXX-${keyStr.slice(-8)}`,
          activation_limit: limit, disabled, expires_at: expires, test_mode: test, created_at: iso(createdAt + 60_000), updated_at: iso(createdAt + 60_000),
        });
        const maxInst = limit === null ? 3 : limit;
        const count = disabled || r.chance(0.3) ? 0 : r.int(1, Math.max(1, maxInst));
        for (let k = 0; k < count; k++) {
          add('instances', { license_key_id: key.id, identifier: r.uuid(), name: instNames(), created_at: iso(createdAt + (k + 1) * DAY), updated_at: iso(createdAt + (k + 1) * DAY) });
        }
      }
    }
    return order;
  }

  const live = db.customers.filter((c) => !c.test_mode && c.id !== demo.id);
  const testCustomers = db.customers.filter((c) => c.test_mode);
  const liveVariantsByStore = (sid) => db.variants.filter((v) => !v.test_mode && productOfVariant(v).store_id === sid && v.status !== 'draft' && priceOfVariant(v));
  const testVariants = [V13, V14];

  // Demo customer: 4 orders (license key, file download, subscription, course w/ links)
  createOrder({ customer: demo, variants: [V1], status: 'paid', createdAt: now - 20 * DAY, source: 'stripe', test: false });
  createOrder({ customer: demo, variants: [V5], status: 'paid', createdAt: now - 35 * DAY, source: 'stripe', test: false });
  createOrder({ customer: demo, variants: [V3], status: 'paid', createdAt: now - 12 * DAY, source: 'stripe', test: false, forceSubStatus: 'active' });
  createOrder({ customer: demo, variants: [V7], status: 'paid', createdAt: now - 58 * DAY, source: 'stripe', test: false });

  // ~136 more orders, spread over 90 days (more recent = denser)
  const makeSpecs = [];
  for (let i = 0; i < 136; i++) {
    const age = Math.floor(Math.pow(r.next(), 1.35) * 90 * DAY) + r.int(0, 20) * HOUR;
    makeSpecs.push({ age, test: r.chance(0.15) });
  }
  makeSpecs.sort((a, b) => b.age - a.age);
  for (const spec of makeSpecs) {
    const status = r.weighted([['paid', 76], ['refunded', 6], ['partial_refund', 5], ['failed', 4], ['pending', 4], ['fraudulent', 1]]);
    const source = r.weighted([['stripe', 80], ['lemonsqueezy', 14], ['manual', 6]]);
    let customer;
    let pool;
    if (spec.test) {
      customer = r.pick(testCustomers);
      pool = testVariants;
    } else {
      customer = r.pick(live);
      pool = liveVariantsByStore(customer.store_id);
    }
    const first = r.pick(pool);
    const variants = [first];
    if (!spec.test && r.chance(0.18)) {
      const second = r.pick(pool);
      if (second.id !== first.id) variants.push(second);
    }
    createOrder({ customer, variants, status, createdAt: now - spec.age, source, test: spec.test });
  }
  // Order numbers follow creation time
  db.orders.sort((a, b) => a.created_at.localeCompare(b.created_at));
  db.orders.forEach((o, i) => (o.order_number = 1001 + i));
  db.orders.sort((a, b) => a.id - b.id);
  db.seq.orders = db.orders.length;

  // ------------------------------------------------------------------ webhooks + deliveries
  const mkWebhook = (store, url, events, test = false) =>
    add('webhooks', { store_id: storeOf(store), url, events, disabled: false, secret_set: true, test_mode: test, created_at: iso(now - 70 * DAY), updated_at: iso(now - 5 * DAY) });
  const W1 = mkWebhook(0, 'https://hooks.acme-software.dev/df/fulfillment', ['order_created', 'order_refunded', 'subscription_created', 'subscription_cancelled', 'subscription_expired', 'license_key_created']);
  const W2 = mkWebhook(0, 'https://erp.acme-software.dev/ingest/orders', ['order_created', 'order_refunded']);
  const W3 = mkWebhook(1, 'https://hooks.zapier.com/hooks/catch/1234567/abcdef/', ['license_key_created', 'license_key_updated', 'subscription_payment_failed']);
  W3.disabled = false;
  const WT = mkWebhook(0, 'https://staging.acme-software.dev/hooks/df', ['order_created', 'license_key_created', 'subscription_created'], true);
  const candidates = [];
  for (const o of db.orders) {
    const at = Date.parse(o.created_at) + 1000;
    if (['paid', 'partial_refund', 'refunded'].includes(o.status)) candidates.push({ event: 'order_created', type: 'orders', rid: o.id, at, test: o.test_mode, store: o.store_id });
    if (o.status === 'refunded') candidates.push({ event: 'order_refunded', type: 'orders', rid: o.id, at: Date.parse(o.refunded_at), test: o.test_mode, store: o.store_id });
  }
  for (const s of db.subscriptions) candidates.push({ event: 'subscription_created', type: 'subscriptions', rid: s.id, at: Date.parse(s.created_at) + 2000, test: s.test_mode, store: s.store_id });
  for (const k of db.licenseKeys) candidates.push({ event: 'license_key_created', type: 'license-keys', rid: k.id, at: Date.parse(k.created_at) + 3000, test: k.test_mode, store: k.store_id });
  candidates.sort((a, b) => b.at - a.at);
  const liveCandidates = candidates.filter((c) => !c.test).slice(0, 54);
  const testCandidates = candidates.filter((c) => c.test).slice(0, 8);
  for (const c of [...liveCandidates, ...testCandidates]) {
    const subs = db.webhooks.filter((w) => w.test_mode === c.test && w.events.includes(c.event));
    if (!subs.length) continue;
    const w = r.pick(subs);
    const status = r.weighted([['success', 70], ['failed', 18], ['pending', 12]]);
    const payload = JSON.stringify({ meta: { test_mode: c.test, event_name: c.event, webhook_id: String(w.id) }, data: { type: c.type, id: String(c.rid), attributes: { store_id: c.store, created_at: iso(c.at) } } }, null, 2);
    const dur = r.int(80, 900);
    add('deliveries', {
      webhook_id: w.id, webhook_url: w.url, event_name: c.event, resource_type: c.type, resource_id: c.rid, status,
      attempts: status === 'success' ? 1 : status === 'failed' ? r.int(3, 5) : r.int(1, 2), next_attempt_at: status === 'pending' ? iso(now + r.int(1, 30) * 60_000) : null,
      last_status_code: status === 'success' ? 200 : status === 'failed' ? r.pick([500, 502, 404, 410]) : null,
      last_response: status === 'success' ? '{"ok":true}' : status === 'failed' ? 'Internal Server Error' : '', last_error: status === 'failed' && r.chance(0.4) ? 'dial tcp: connection refused' : '',
      duration_ms: status === 'pending' ? 0 : dur, created_at: iso(Math.min(now, c.at)), delivered_at: status === 'success' ? iso(Math.min(now, c.at + dur)) : null, _payload: payload,
    });
  }
  db.deliveries.sort((a, b) => a.created_at.localeCompare(b.created_at));
  db.deliveries.forEach((d, i) => (d.id = i + 1));
  db.seq.deliveries = db.deliveries.length;

  // ------------------------------------------------------------------ stripe events
  const evTypes = [
    ['checkout.session.completed', 30], ['invoice.paid', 22], ['customer.subscription.updated', 14], ['charge.refunded', 6],
    ['invoice.payment_failed', 6], ['customer.subscription.deleted', 5], ['payment_intent.succeeded', 10], ['customer.created', 7],
  ];
  for (let i = 0; i < 40; i++) {
    const type = r.weighted(evTypes);
    const status = type === 'customer.created' || type === 'payment_intent.succeeded' ? r.weighted([['ignored', 70], ['processed', 30]]) : r.weighted([['processed', 78], ['failed', 12], ['received', 10]]);
    const at = now - Math.floor(Math.pow(r.next(), 1.5) * 30 * DAY);
    const livemode = !r.chance(0.15);
    const evtId = `evt_${r.hex(24)}`;
    add('stripeEvents', {
      type, account: 'acct_1MockOwner01', livemode, status, attempts: status === 'failed' ? r.int(2, 6) : status === 'received' ? 0 : 1,
      error: status === 'failed' ? r.pick(['no order found for checkout session cs_live_x', 'stripe: rate limited while fetching invoice', 'database: deadlock detected (retry)']) : '',
      received_at: iso(at), processed_at: status === 'received' ? null : iso(at + r.int(100, 3000)),
      _payload: { id: evtId, object: 'event', api_version: '2025-09-30.clover', created: Math.floor(at / 1000), type, livemode, data: { object: { id: `${type.startsWith('invoice') ? 'in' : type.startsWith('charge') ? 'ch' : type.startsWith('customer.sub') ? 'sub' : type.startsWith('customer') ? 'cus' : 'cs'}_${r.hex(18)}`, object: type.split('.')[0], amount_total: r.int(9, 200) * 100, currency: 'usd', livemode } }, request: { id: `req_${r.hex(14)}`, idempotency_key: r.uuid() } },
    });
  }
  db.stripeEvents.sort((a, b) => a.received_at.localeCompare(b.received_at));
  db.stripeEvents.forEach((e, i) => (e.id = i + 1));
  // guarantee a few failed / unprocessed live events so the "failed events" warning and replay can be exercised
  const eligible = db.stripeEvents.filter((e) => e.livemode && !['customer.created', 'payment_intent.succeeded'].includes(e.type));
  [2, 7, 12].forEach((i) => {
    const e = eligible[i];
    if (e) Object.assign(e, { status: 'failed', attempts: 3, error: 'no order found for checkout session cs_live_unknown', processed_at: e.received_at });
  });
  [4, 9].forEach((i) => {
    const e = eligible[i];
    if (e) Object.assign(e, { status: 'received', attempts: 0, error: '', processed_at: null });
  });
  db.seq.stripeEvents = db.stripeEvents.length;

  // ------------------------------------------------------------------ email logs
  const recentPaid = db.orders.filter((o) => o.status === 'paid' && !o.test_mode).slice(-26);
  for (const o of recentPaid) {
    add('emailLogs', { order_id: o.id, to: o.user_email, subject: `Your Acme order receipt #${o.order_number}`, template: 'order_receipt', status: 'sent', error: '', created_at: iso(Date.parse(o.created_at) + 5000) });
  }
  add('emailLogs', { order_id: null, to: 'customer@example.com', subject: 'Your sign-in link', template: 'magic_link', status: 'sent', error: '', created_at: iso(now - 3 * HOUR) });
  add('emailLogs', { order_id: recentPaid[0].id, to: recentPaid[0].user_email, subject: `Your Acme order receipt #${recentPaid[0].order_number}`, template: 'order_receipt', status: 'failed', error: 'smtp: connection refused (host not configured)', created_at: iso(now - 26 * HOUR) });
  add('emailLogs', { order_id: null, to: 'owner@example.com', subject: 'Payment failed for your subscription', template: 'subscription_payment_failed', status: 'failed', error: 'smtp: 535 authentication failed', created_at: iso(now - 50 * HOUR) });
  db.emailLogs.sort((a, b) => a.created_at.localeCompare(b.created_at));
  db.emailLogs.forEach((e, i) => (e.id = i + 1));
  db.seq.emailLogs = db.emailLogs.length;

  // ------------------------------------------------------------------ audit logs
  const actions = [
    ['owner@example.com', 'product.update', 'products', P1.id], ['owner@example.com', 'variant.create', 'variants', V12.id], ['admin@example.com', 'order.refund', 'orders', db.orders[db.orders.length - 3].id],
    ['admin@example.com', 'license_key.update', 'license-keys', 3], ['owner@example.com', 'settings.update', 'settings', null], ['owner@example.com', 'api_key.create', 'api-keys', 1],
    ['admin@example.com', 'webhook.create', 'webhooks', W1.id], ['owner@example.com', 'user.create', 'users', 3], ['admin@example.com', 'subscription.cancel', 'subscriptions', 4], ['owner@example.com', 'import.lemonsqueezy', 'imports', 1],
  ];
  for (let i = 0; i < 30; i++) {
    const [email, action, type, rid] = actions[i % actions.length];
    add('auditLogs', { user_email: email, action, resource_type: type, resource_id: rid, details: { note: 'seeded', index: i }, ip: i % 4 === 0 ? '203.0.113.7' : '198.51.100.23', created_at: iso(now - (i * 7 + r.int(0, 5)) * HOUR) });
  }
  db.auditLogs.reverse();
  db.auditLogs.forEach((a, i) => (a.id = i + 1));
  db.seq.auditLogs = db.auditLogs.length;

  // ------------------------------------------------------------------ api keys
  add('apiKeys', { name: 'Production sync', prefix: 'dfk_live_a1b2', test_mode: false, owner_email: 'owner@example.com', last_used_at: iso(now - 2 * HOUR), expires_at: null, revoked_at: null, created_at: iso(now - 80 * DAY) });
  add('apiKeys', { name: 'CI (test mode)', prefix: 'dfk_test_c3d4', test_mode: true, owner_email: 'admin@example.com', last_used_at: iso(now - 9 * DAY), expires_at: iso(now + 90 * DAY), revoked_at: null, created_at: iso(now - 30 * DAY) });

  // ------------------------------------------------------------------ stripe catalog
  db.catalog = [
    { id: 'prod_MockPixel01', name: 'Pixel Studio', active: true, description: 'Professional pixel-art editor', metadata: {}, prices: [{ id: 'price_MockPixelPersonal', unit_amount: 4900, currency: 'usd', recurring: null, active: true }, { id: 'price_MockPixelTeam', unit_amount: 19900, currency: 'usd', recurring: null, active: true }] },
    { id: 'prod_MockNotekit01', name: 'Notekit', active: true, description: 'Local-first notes', metadata: { plan: 'pro' }, prices: [{ id: 'price_MockNotekitMonthly', unit_amount: 900, currency: 'usd', recurring: { interval: 'month', interval_count: 1 }, active: true }, { id: 'price_MockNotekitYearly', unit_amount: 9000, currency: 'usd', recurring: { interval: 'year', interval_count: 1 }, active: true }] },
    { id: 'prod_MockIcons01', name: 'Line UI Icon Pack', active: true, description: '', metadata: {}, prices: [{ id: 'price_MockIconsStandard', unit_amount: 2900, currency: 'usd', recurring: null, active: true }] },
    { id: 'prod_MockLumen01', name: 'Lumen Typeface', active: true, description: 'Desktop font license', metadata: {}, prices: [{ id: 'price_MockLumenDesktop', unit_amount: 5900, currency: 'usd', recurring: null, active: true }] },
    { id: 'prod_MockLegacy01', name: 'Legacy Plugin (v1)', active: true, description: 'Old WordPress plugin, still sold on a few sites', metadata: {}, prices: [{ id: 'price_MockLegacyOnce', unit_amount: 3900, currency: 'usd', recurring: null, active: true }] },
    { id: 'prod_MockSupport01', name: 'Priority Support', active: true, description: 'Monthly priority support plan', metadata: {}, prices: [{ id: 'price_MockSupportMonthly', unit_amount: 2900, currency: 'usd', recurring: { interval: 'month', interval_count: 1 }, active: true }, { id: 'price_MockSupportOld', unit_amount: 1900, currency: 'usd', recurring: { interval: 'month', interval_count: 1 }, active: false }] },
  ].map((p) => ({ ...p, default_price: p.prices[0].id }));

  // ------------------------------------------------------------------ settings
  db.settings = {
    urls: {
      public_url: PUBLIC_URL, stripe_webhook_url: `${PUBLIC_URL}/webhooks/stripe`,
      stripe_oauth_redirect_uri: `${PUBLIC_URL}/admin-api/auth/stripe/callback`, api_base_url: `${PUBLIC_URL}/v1`,
    },
    stripe: {
      secret_key: { set: true, source: 'db', hint: 'sk_live_…1234' }, webhook_secret: { set: true, source: 'env', hint: 'whsec_…ab12' },
      api_version: '2025-09-30.clover', managed_payments: false, oauth_client_id: 'ca_MockClient0001', oauth_secret_key: { set: false },
      login_mode: 'allowlist', login_accounts: ['acct_1MockOwner01'], login_auto_role: 'viewer', oauth_revoke_after_login: true,
    },
    smtp: { host: '', port: 587, user: '', password: { set: false }, from: '', security: 'starttls' },
    license: { disable_on_refund: true },
    download: { limit_per_day: 10, url_ttl_seconds: 3600 },
    checkout: { success_url: '', cancel_url: '' },
  };

  // ------------------------------------------------------------------ imports
  const mkLog = (jobId, level, message, at) => add('importLogs', { job_id: jobId, level, message, created_at: iso(at) });
  const j1Start = now - 3 * DAY;
  const j1 = add('imports', {
    kind: 'lemonsqueezy_api', status: 'completed', options: { test_mode: false, store_ids: [], resources: ['stores', 'products', 'variants', 'prices', 'files', 'customers', 'orders', 'subscriptions', 'license_keys'], download_files: true, dry_run: false },
    progress: { stage: 'license_keys', done: 90, total: 90, counts: { stores: 2, products: 7, variants: 12, prices: 12, files: 12, customers: 45, orders: 140, subscriptions: 25, license_keys: 90 }, warnings: 3 },
    error: null, created_by: 'owner@example.com', started_at: iso(j1Start + 2000), finished_at: iso(j1Start + 4 * 60_000), created_at: iso(j1Start),
  });
  ['Starting Lemon Squeezy import', 'Fetched 2 stores', 'Imported 7 products', 'Imported 12 variants and 12 prices', 'Downloaded 12 files (1.0 GB)', 'Imported 45 customers', 'Imported 140 orders'].forEach((m, i) => mkLog(j1.id, 'info', m, j1Start + (i + 1) * 20_000));
  mkLog(j1.id, 'warn', 'Order #1033: no Stripe payment intent found (refunds will need manual handling)', j1Start + 3 * 60_000);
  mkLog(j1.id, 'info', 'Import completed with 3 warnings', j1Start + 4 * 60_000);
  const j2Start = now - 1 * DAY;
  const j2 = add('imports', {
    kind: 'lemonsqueezy_archive', status: 'failed', options: { download_files: false, filename: 'ls-export-2026-10.zip' },
    progress: { stage: 'products', done: 0, total: 0, counts: {}, warnings: 0 }, error: 'archive is missing products.json (is this a Lemon Squeezy export?)',
    created_by: 'admin@example.com', started_at: iso(j2Start + 1000), finished_at: iso(j2Start + 3000), created_at: iso(j2Start),
  });
  mkLog(j2.id, 'info', 'Reading archive ls-export-2026-10.zip', j2Start + 1000);
  mkLog(j2.id, 'error', 'archive is missing products.json (is this a Lemon Squeezy export?)', j2Start + 3000);
  const j3 = add('imports', {
    kind: 'lemonsqueezy_api', status: 'running', options: { test_mode: false, store_ids: [], resources: ['stores', 'products', 'variants', 'prices', 'customers', 'orders', 'subscriptions', 'license_keys'], download_files: false, dry_run: true },
    progress: { stage: 'stores', done: 0, total: 2, counts: {}, warnings: 0 }, error: null, created_by: 'owner@example.com', started_at: iso(now - 1000), finished_at: null, created_at: iso(now - 2500),
    _t0: now - 2500, _duration: 30_000, _stages: [['stores', 2], ['products', 7], ['variants', 12], ['prices', 12], ['customers', 45], ['orders', 140], ['subscriptions', 25], ['license_keys', 90]], _lastTick: 0, _fail: false,
  });
  mkLog(j3.id, 'info', '[dry-run] Starting Lemon Squeezy import (nothing will be written)', now - 2000);

  // ------------------------------------------------------------------ finishing touches
  db.orders.forEach((o) => {
    // keep references stable
    o.updated_at ||= o.created_at;
  });
  db.helpers = { variantById, productOfVariant, priceOfVariant, storeIds };
  return db;
}
