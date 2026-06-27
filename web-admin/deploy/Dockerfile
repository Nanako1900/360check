# syntax=docker/dockerfile:1
# C5 web-admin SPA → static files served by nginx-unprivileged (non-root).
# Build context is the web-admin/ directory.
#
# Vite 在 build 时把 import.meta.env.VITE_* 内联进静态产物（属【公开值】）。Coolify 注入的
# build ARG 只在 Dockerfile 顶部的【第一个 build stage】生效；多阶段时必须在跑 `npm run build`
# 的这个 stage 内部【再次声明 ARG】，否则 Vite 拿不到值。对应 Coolify 变量须勾选 "Build Variable"。
FROM node:22-alpine AS builder
WORKDIR /app

# —— 构建期注入（Coolify「Build Variable」/ compose build.args）——
# 默认值保证「零注入」也能产出可用镜像（API 同源 /api/v1、MSW 关闭）。
ARG VITE_API_BASE_URL=/api/v1
ARG VITE_MAP_KEY=
ARG VITE_CDN_BASE=
ARG VITE_ENABLE_MSW=false
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_MAP_KEY=$VITE_MAP_KEY \
    VITE_CDN_BASE=$VITE_CDN_BASE \
    VITE_ENABLE_MSW=$VITE_ENABLE_MSW

COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# `build` 先 tsc --noEmit 再 vite build（类型错误即让镜像构建失败）。
# 生产模式 import.meta.env.DEV=false → MSW(./mocks/browser, ~300KB) 整段被静态消除，不进产物。
RUN npm run build

# nginx-unprivileged 监听 8080，以 uid 101（非 root）运行 → 可配 runAsNonRoot +
# readOnlyRootFilesystem。仅本服务对外暴露公网域名，并把 /api 同源反代到内部 api。
FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 8080
