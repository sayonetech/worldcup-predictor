# syntax=docker/dockerfile:1
# Multi-stage: build the Vite bundle with Node, serve static assets via nginx
# (nginx also reverse-proxies /api to the backend — see nginx.conf).

FROM node:22-alpine AS build
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml .npmrc ./
# The game bundle is a vendored local tarball referenced via `file:` in
# package.json, so it must be present in the build context BEFORE install
# resolves the lockfile (otherwise pnpm install fails with ENOENT).
COPY goat-game ./goat-game
RUN pnpm install --frozen-lockfile
COPY . .
# Vite inlines VITE_* at BUILD time, so they must be present here (not at runtime).
ARG VITE_GOOGLE_CLIENT_ID=""
ARG VITE_API_BASE_URL="/api"
ENV VITE_GOOGLE_CLIENT_ID=$VITE_GOOGLE_CLIENT_ID
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
RUN pnpm build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
