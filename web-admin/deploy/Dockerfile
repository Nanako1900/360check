# syntax=docker/dockerfile:1
# C5 web-admin SPA → static files served by nginx-unprivileged (non-root).
# Build context is the web-admin/ directory.
FROM node:22-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# `build` runs tsc --noEmit then vite build (fails the image on a type error).
RUN npm run build

# nginx-unprivileged listens on 8080 and runs as uid 101 (non-root), so the k8s
# pod can use runAsNonRoot + readOnlyRootFilesystem.
FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 8080
