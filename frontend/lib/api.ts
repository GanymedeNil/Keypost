/**
 * Typed API client for the Go backend (same origin, proxied by next.config rewrites).
 *  - adds the CSRF header on every mutating request
 *  - JSON in/out, `ApiError` carrying `{code,message,fields}`
 *  - 401 on the admin API redirects to /admin/login?next=…
 */
import type { Paginated } from './types';

export const ADMIN_BASE = '/admin-api';
export const STOREFRONT_BASE = '/storefront-api';
const CSRF_HEADER = 'X-Requested-With';
const CSRF_VALUE = 'df-admin';

export type QueryValue = string | number | boolean | null | undefined | readonly (string | number)[];
export type Query = Record<string, QueryValue>;

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields: Record<string, string>;

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

/** Best-effort message for any thrown value. */
export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
}

export function buildQuery(query?: Query): string {
  if (!query) return '';
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === '') continue;
    if (Array.isArray(v)) {
      for (const item of v) sp.append(k, String(item));
    } else {
      sp.set(k, String(v));
    }
  }
  const s = sp.toString();
  return s ? `?${s}` : '';
}

interface RequestOptions {
  method?: string;
  query?: Query;
  body?: unknown;
  signal?: AbortSignal;
  /** Set false to disable the automatic redirect to the login page on 401. */
  redirectOn401?: boolean;
}

function redirectToLogin() {
  if (typeof window === 'undefined') return;
  const { pathname, search, origin } = window.location;
  if (pathname.startsWith('/admin/login')) return;
  const target = new URL('/admin/login', origin);
  target.searchParams.set('next', pathname + search);
  window.location.assign(target);
}

function redirectToMfa() {
  if (typeof window === 'undefined') return;
  const { pathname, search, origin } = window.location;
  if (pathname.startsWith('/admin/mfa') || pathname.startsWith('/admin/login')) return;
  const target = new URL('/admin/mfa', origin);
  target.searchParams.set('next', pathname + search);
  window.location.assign(target);
}

async function parseError(res: Response): Promise<ApiError> {
  let code = 'internal';
  let message = res.statusText || `HTTP ${res.status}`;
  let fields: Record<string, string> = {};
  try {
    const data = (await res.json()) as { error?: { code?: string; message?: string; fields?: Record<string, string> } };
    if (data?.error) {
      code = data.error.code ?? code;
      message = data.error.message ?? message;
      fields = data.error.fields ?? {};
    }
  } catch {
    /* non-JSON error body */
  }
  return new ApiError(res.status, code, message, fields);
}

export async function request<T>(url: string, opts: RequestOptions = {}): Promise<T> {
  const method = (opts.method ?? 'GET').toUpperCase();
  const headers: Record<string, string> = { Accept: 'application/json' };
  let body: BodyInit | undefined;
  if (method !== 'GET' && method !== 'HEAD') headers[CSRF_HEADER] = CSRF_VALUE;
  if (opts.body instanceof FormData) {
    body = opts.body;
  } else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
    body = JSON.stringify(opts.body);
  }
  let res: Response;
  try {
    res = await fetch(url + buildQuery(opts.query), {
      method,
      headers,
      body,
      signal: opts.signal,
      credentials: 'same-origin',
      cache: 'no-store',
    });
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') throw e;
    throw new ApiError(0, 'network_error', e instanceof Error ? e.message : 'Network error');
  }
  if (!res.ok) {
    const err = await parseError(res);
    if (res.status === 401 && opts.redirectOn401 !== false && url.startsWith(ADMIN_BASE) && !url.startsWith(`${ADMIN_BASE}/auth/`)) {
      redirectToLogin();
    }
    if (res.status === 403 && (err.code === 'mfa_required' || err.code === 'mfa_setup_required') && url.startsWith(ADMIN_BASE)) {
      redirectToMfa();
    }
    throw err;
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export interface UploadOptions {
  onProgress?: (loaded: number, total: number) => void;
  signal?: AbortSignal;
}

/** multipart upload with progress (fetch cannot report upload progress, XHR can). */
export function upload<T>(url: string, form: FormData, opts: UploadOptions = {}): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url);
    xhr.setRequestHeader(CSRF_HEADER, CSRF_VALUE);
    xhr.setRequestHeader('Accept', 'application/json');
    xhr.withCredentials = true;
    xhr.upload.onprogress = (ev) => {
      if (ev.lengthComputable) opts.onProgress?.(ev.loaded, ev.total);
    };
    xhr.onerror = () => reject(new ApiError(0, 'network_error', 'Network error'));
    xhr.onabort = () => reject(new DOMException('Aborted', 'AbortError'));
    xhr.onload = () => {
      const text = xhr.responseText;
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve((text ? JSON.parse(text) : undefined) as T);
        } catch {
          resolve(undefined as T);
        }
        return;
      }
      if (xhr.status === 401 && url.startsWith(ADMIN_BASE)) redirectToLogin();
      let code = 'internal';
      let message = xhr.statusText || `HTTP ${xhr.status}`;
      let fields: Record<string, string> = {};
      try {
        const data = JSON.parse(text) as { error?: { code?: string; message?: string; fields?: Record<string, string> } };
        code = data.error?.code ?? code;
        message = data.error?.message ?? message;
        fields = data.error?.fields ?? {};
      } catch {
        if (xhr.status === 413) {
          code = 'payload_too_large';
          message = 'File too large';
        }
      }
      reject(new ApiError(xhr.status, code, message, fields));
    };
    if (opts.signal) {
      if (opts.signal.aborted) {
        reject(new DOMException('Aborted', 'AbortError'));
        return;
      }
      opts.signal.addEventListener('abort', () => xhr.abort(), { once: true });
    }
    xhr.send(form);
  });
}

function makeClient(base: string) {
  const full = (path: string) => `${base}${path}`;
  return {
    base,
    /** Absolute (same-origin) URL for links / downloads / SWR keys. */
    url: (path: string, query?: Query) => `${full(path)}${buildQuery(query)}`,
    get: <T>(path: string, query?: Query, opts?: Omit<RequestOptions, 'method' | 'query' | 'body'>) =>
      request<T>(full(path), { ...opts, query }),
    list: <T>(path: string, query?: Query, opts?: Omit<RequestOptions, 'method' | 'query' | 'body'>) =>
      request<Paginated<T>>(full(path), { ...opts, query }),
    post: <T = void>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
      request<T>(full(path), { ...opts, method: 'POST', body: body ?? {} }),
    put: <T = void>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
      request<T>(full(path), { ...opts, method: 'PUT', body: body ?? {} }),
    patch: <T = void>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
      request<T>(full(path), { ...opts, method: 'PATCH', body: body ?? {} }),
    delete: <T = void>(path: string, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
      request<T>(full(path), { ...opts, method: 'DELETE' }),
    upload: <T>(path: string, form: FormData, opts?: UploadOptions) => upload<T>(full(path), form, opts),
  };
}

/** `/admin-api/*` */
export const adminApi = makeClient(ADMIN_BASE);
/** `/storefront-api/*` (customer facing; never redirects to the admin login). */
export const storefrontApi = makeClient(STOREFRONT_BASE);

/** SWR fetcher: the key is the full URL (with query string). */
export const fetcher = <T,>(url: string): Promise<T> => request<T>(url);
