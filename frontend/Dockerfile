# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Keypost frontend (Next.js 16, standalone output, non-root).
#
#   docker build --build-arg BACKEND_URL=http://backend:8080 -t df-frontend .
#   docker run -p 3000:3000 df-frontend
#
# BACKEND_URL: the Go API origin. IMPORTANT: Next.js evaluates `rewrites()` at
# BUILD time and bakes the destination into .next/routes-manifest.json, so
# BACKEND_URL must be passed as a build arg (it is NOT read again at runtime).
# Rebuild the image to point it at a different backend.
# ---------------------------------------------------------------------------
ARG NODE_VERSION=24

FROM node:${NODE_VERSION}-alpine AS base
ENV PNPM_HOME=/pnpm \
    PATH=/pnpm:$PATH \
    NEXT_TELEMETRY_DISABLED=1
RUN corepack enable
WORKDIR /app

FROM base AS deps
COPY package.json pnpm-lock.yaml ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile

FROM base AS build
ARG BACKEND_URL=http://backend:8080
ENV BACKEND_URL=${BACKEND_URL}
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# `postbuild` copies .next/static and public into .next/standalone
RUN pnpm build

FROM node:${NODE_VERSION}-alpine AS runner
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0
WORKDIR /app
RUN addgroup -S -g 1001 app && adduser -S -u 1001 -G app app
COPY --from=build --chown=app:app /app/.next/standalone ./
USER app
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://127.0.0.1:3000/admin/login || exit 1
CMD ["node", "server.js"]
