# syntax=docker/dockerfile:1
FROM node:26.13.1-bookworm-slim AS build
ARG BUN_VERSION=1.4.4
ENV NEXT_TELEMETRY_DISABLED=1
WORKDIR /app/frontend
RUN npm install --global --no-audit --no-fund --allow-scripts=bun "bun@${BUN_VERSION}"
COPY frontend/package.json frontend/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache,id=project-creator-bun-${BUN_VERSION} bun install --frozen-lockfile
COPY contracts /app/contracts
COPY graph /app/graph
COPY frontend /app/frontend
RUN bun run codegen && bun run build

FROM node:26.13.1-bookworm-slim AS runtime
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
WORKDIR /app
COPY --from=build --chown=node:node /app/frontend/.next/standalone ./
COPY --from=build --chown=node:node /app/frontend/.next/static ./frontend/.next/static
USER node
EXPOSE 3000
CMD ["node", "frontend/server.js"]
