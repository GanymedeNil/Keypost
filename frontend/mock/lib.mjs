// Shared helpers for the mock backend: errors, PRNG, pagination/sorting, multipart parsing, CSV.
import crypto from 'node:crypto';

export const HOUR = 3_600_000;
export const DAY = 24 * HOUR;

// ---------------------------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------------------------
export class ApiError extends Error {
  constructor(status, code, message, fields) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

export const E = {
  notFound: (what = 'resource') => new ApiError(404, 'not_found', `${what} not found`),
  forbidden: (m = 'You do not have permission to perform this action') => new ApiError(403, 'forbidden', m),
  unauth: (m = 'Authentication required') => new ApiError(401, 'unauthenticated', m),
  invalid: (fields, message = 'Validation failed') => new ApiError(422, 'validation_failed', message, fields),
  badRequest: (m = 'Bad request') => new ApiError(400, 'validation_failed', m),
  conflict: (m) => new ApiError(409, 'conflict', m),
  stripe: (m) => new ApiError(502, 'stripe_error', m),
};

// ---------------------------------------------------------------------------------------------
// Time / formatting
// ---------------------------------------------------------------------------------------------
export const iso = (ms) => new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z');
export const isoDate = (ms) => new Date(ms).toISOString().slice(0, 10);

export function money(cents, currency = 'USD') {
  try {
    return new Intl.NumberFormat('en-US', { style: 'currency', currency }).format((cents ?? 0) / 100);
  } catch {
    return `${((cents ?? 0) / 100).toFixed(2)} ${currency}`;
  }
}

export function humanSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1).replace(/\.0$/, '')} ${units[i]}`;
}

export const capitalize = (s) => (s ? s.charAt(0).toUpperCase() + s.slice(1).replace(/_/g, ' ') : s);
export const slugify = (s) =>
  String(s || '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '') || 'item';

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
export const randomUuid = () => crypto.randomUUID();
export const randomHex = (n) => crypto.randomBytes(Math.ceil(n / 2)).toString('hex').slice(0, n);

// ---------------------------------------------------------------------------------------------
// Seeded PRNG
// ---------------------------------------------------------------------------------------------
export function makeRng(seed) {
  let a = seed >>> 0;
  const next = () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
  const int = (lo, hi) => lo + Math.floor(next() * (hi - lo + 1));
  const pick = (arr) => arr[Math.floor(next() * arr.length)];
  const chance = (p) => next() < p;
  const hex = (n) => Array.from({ length: n }, () => Math.floor(next() * 16).toString(16)).join('');
  const uuid = () => {
    const h = hex(32);
    return `${h.slice(0, 8)}-${h.slice(8, 12)}-4${h.slice(13, 16)}-8${h.slice(17, 20)}-${h.slice(20)}`;
  };
  const weighted = (entries) => {
    const total = entries.reduce((s, [, w]) => s + w, 0);
    let x = next() * total;
    for (const [v, w] of entries) {
      if ((x -= w) < 0) return v;
    }
    return entries[entries.length - 1][0];
  };
  return { next, int, pick, chance, hex, uuid, weighted };
}

// ---------------------------------------------------------------------------------------------
// Lists
// ---------------------------------------------------------------------------------------------
export function paginate(list, q) {
  const per = Math.min(100, Math.max(1, parseInt(q.per_page, 10) || 25));
  const page = Math.max(1, parseInt(q.page, 10) || 1);
  const total = list.length;
  return {
    data: list.slice((page - 1) * per, page * per),
    meta: { page, per_page: per, total, last_page: Math.max(1, Math.ceil(total / per)) },
  };
}

export function sortList(list, sort) {
  if (!sort) return list;
  const out = [...list];
  const keys = sort.split(',').filter(Boolean).map((s) => (s.startsWith('-') ? [s.slice(1), -1] : [s.replace(/^\+/, ''), 1]));
  out.sort((x, y) => {
    for (const [k, dir] of keys) {
      const a = x[k];
      const b = y[k];
      if (a == null && b == null) continue;
      if (a == null) return 1;
      if (b == null) return -1;
      let c;
      if (typeof a === 'number' && typeof b === 'number') c = a - b;
      else if (typeof a === 'boolean' && typeof b === 'boolean') c = Number(a) - Number(b);
      else c = String(a).localeCompare(String(b));
      if (c !== 0) return c * dir;
    }
    return 0;
  });
  return out;
}

export function matchQ(item, q, fields) {
  if (!q) return true;
  const needle = String(q).toLowerCase();
  return fields.some((f) => {
    const v = typeof f === 'function' ? f(item) : item[f];
    return v != null && String(v).toLowerCase().includes(needle);
  });
}

export function csvEscape(v) {
  if (v == null) return '';
  const s = String(v);
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}
export const toCsv = (header, rows) => [header, ...rows].map((r) => r.map(csvEscape).join(',')).join('\n') + '\n';

// ---------------------------------------------------------------------------------------------
// Request body helpers
// ---------------------------------------------------------------------------------------------
export async function readJson(req) {
  const chunks = [];
  for await (const c of req) chunks.push(c);
  const text = Buffer.concat(chunks).toString('utf8').trim();
  if (!text) return {};
  try {
    const v = JSON.parse(text);
    return v && typeof v === 'object' ? v : {};
  } catch {
    throw E.badRequest('Request body is not valid JSON');
  }
}

const FULL_PARSE_LIMIT = 32 * 1024 * 1024;
const EDGE = 64 * 1024;

/**
 * Reads a multipart/form-data request. Small bodies (<= 32 MB) are fully buffered and parsed.
 * Larger bodies are streamed: only the first/last 64 KB are retained, which is enough to read the
 * file part headers, the text fields, and to compute the file size.
 * Returns { fields: {name: value}, file: {fieldname, filename, size, data|null} | null }.
 */
export async function readMultipart(req) {
  const ctype = req.headers['content-type'] || '';
  const m = /boundary=(?:"([^"]+)"|([^;]+))/i.exec(ctype);
  if (!/multipart\/form-data/i.test(ctype) || !m) throw E.badRequest('Expected multipart/form-data body');
  const boundary = (m[1] || m[2]).trim();
  const chunks = [];
  let size = 0;
  let head = Buffer.alloc(0);
  let tail = Buffer.alloc(0);
  let big = false;
  for await (const chunk of req) {
    size += chunk.length;
    if (!big) {
      chunks.push(chunk);
      if (size > FULL_PARSE_LIMIT) {
        big = true;
        const all = Buffer.concat(chunks);
        head = all.subarray(0, EDGE);
        tail = all.subarray(all.length - EDGE);
        chunks.length = 0;
      }
    } else {
      tail = Buffer.concat([tail, chunk]);
      if (tail.length > EDGE) tail = tail.subarray(tail.length - EDGE);
    }
  }
  if (!big) return parseMultipart(Buffer.concat(chunks), null, size, boundary);
  return parseMultipart(head, tail, size, boundary);
}

function segments(buf, delim) {
  const pos = [];
  let i = buf.indexOf(delim);
  while (i !== -1) {
    pos.push(i);
    i = buf.indexOf(delim, i + delim.length);
  }
  return pos;
}

function parseHeaders(raw) {
  const out = {};
  for (const line of raw.split('\r\n')) {
    const idx = line.indexOf(':');
    if (idx > 0) out[line.slice(0, idx).trim().toLowerCase()] = line.slice(idx + 1).trim();
  }
  const disp = out['content-disposition'] || '';
  const name = /\bname="([^"]*)"/.exec(disp)?.[1];
  const filename = /\bfilename="([^"]*)"/.exec(disp)?.[1];
  return { name, filename };
}

export function parseMultipart(head, tail, total, boundary) {
  const delim = Buffer.from(`--${boundary}`);
  const fields = {};
  let file = null;

  const consume = (buf, absOffset, isHead) => {
    const pos = segments(buf, delim);
    pos.forEach((p, idx) => {
      const afterDelim = p + delim.length;
      if (buf[afterDelim] === 0x2d && buf[afterDelim + 1] === 0x2d) return; // closing delimiter
      const next = pos[idx + 1];
      const complete = next !== undefined;
      if (!isHead && idx === pos.length - 1 && !complete) return;
      const start = afterDelim + 2; // skip CRLF
      const end = complete ? next - 2 : buf.length;
      const seg = buf.subarray(start, end);
      const sep = seg.indexOf('\r\n\r\n');
      if (sep === -1) return;
      const hdr = parseHeaders(seg.subarray(0, sep).toString('utf8'));
      const dataStart = sep + 4;
      if (hdr.filename !== undefined) {
        if (complete) {
          const data = seg.subarray(dataStart);
          file = { fieldname: hdr.name, filename: hdr.filename, size: data.length, data: total <= FULL_PARSE_LIMIT ? data : null };
        } else {
          // truncated (large) file in head: size is resolved after we look at the tail
          file = { fieldname: hdr.name, filename: hdr.filename, size: null, data: null, _startAbs: absOffset + start + dataStart };
        }
      } else if (complete && hdr.name) {
        fields[hdr.name] = seg.subarray(dataStart).toString('utf8');
      }
    });
  };

  consume(head, 0, true);
  if (tail) {
    const tailAbs = total - tail.length;
    const tpos = segments(tail, delim);
    if (file && file.size === null && tpos.length) {
      file.size = Math.max(0, tailAbs + tpos[0] - 2 - file._startAbs);
    }
    if (tpos.length) {
      // parse only the part of the tail that begins at the first delimiter
      consume(tail.subarray(tpos[0]), tailAbs + tpos[0], false);
    }
  }
  if (file && file.size === null) file.size = Math.max(0, total - file._startAbs);
  if (file) delete file._startAbs;
  return { fields, file };
}
