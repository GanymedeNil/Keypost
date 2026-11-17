import type { NextConfig } from 'next';

const backend = (process.env.BACKEND_URL ?? 'http://localhost:8080').replace(/\/+$/, '');

// Everything the Go backend owns is proxied through this Next.js server so the
// browser only ever talks to one origin (cookies, CSRF header, no CORS).
const proxied = [
  '/admin-api/:path*',
  '/storefront-api/:path*',
  '/v1/:path*',
  '/download/:path*',
  '/checkout/:path*',
  '/webhooks/:path*',
  '/billing',
  '/healthz',
];

const nextConfig: NextConfig = {
  output: 'standalone',
  reactStrictMode: true,
  poweredByHeader: false,
  agentRules: false,
  experimental: {
    // Only relevant when proxy.ts runs on a request (it does not match
    // /admin-api/*), kept generous as a safety net for large file uploads.
    proxyClientMaxBodySize: '2gb',
  },
  async rewrites() {
    return {
      beforeFiles: proxied.map((source) => ({
        source,
        destination: `${backend}${source}`,
      })),
    };
  },
};

export default nextConfig;
