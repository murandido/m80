# The panel was built and served by Vercel until this image existed. Self-hosted
# it runs as a container beside the API, so the build has to state what Vercel
# resolved implicitly: the toolchain versions, and the env schema the build
# refuses to run without.

FROM node:22-slim AS base
# The lockfile is 9.0, which does not say whether pnpm 9 or 10 wrote it - Vercel
# resolved that ambiguity to 10 by project creation date. Pinning it here is what
# keeps the resolution from drifting once Vercel is gone.
RUN corepack enable && corepack prepare pnpm@10 --activate
WORKDIR /app

FROM base AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
# Resolved inside the target architecture on purpose: sharp, swc and lightningcss
# ship as per-platform packages, and pnpm-workspace.yaml lists sharp under
# ignoredBuiltDependencies, so there is no build script to fall back on.
RUN pnpm install --frozen-lockfile

FROM base AS builder
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# next.config.ts and src/lib/env.ts both parse the env schema at module load, so
# `next build` fails outright unless both variables exist here. BACKEND_URL is a
# Compose service name, identical in every environment, so baking it in costs
# nothing - Compose passes the same value again at runtime.
ARG BACKEND_URL=http://api-prod:8080
ENV BACKEND_URL=$BACKEND_URL
ENV NODE_ENV=production
# next/font/google fetches the font files during the build, so this stage needs
# outbound network.
RUN pnpm build

FROM base AS runner
ENV NODE_ENV=production
# The server writes nothing, so it has no reason to own its own files.
USER node
# `output: "standalone"` emits a server with only the traced dependencies inlined,
# which is why node_modules is absent here. Static assets are not traced and have
# to be copied separately.
COPY --from=builder --chown=node:node /app/.next/standalone ./
COPY --from=builder --chown=node:node /app/.next/static ./.next/static
ENV PORT=3000
# Without this the standalone server binds to localhost and Caddy cannot reach it
# from its own container.
ENV HOSTNAME=0.0.0.0
EXPOSE 3000
CMD ["node", "server.js"]
