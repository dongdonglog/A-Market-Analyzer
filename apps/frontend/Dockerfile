FROM node:22-bookworm AS builder

WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

COPY . .

ARG VITE_API_BASE_URL=/api
ARG VITE_COPILOT_API_BASE_URL=/api
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
ENV VITE_COPILOT_API_BASE_URL=$VITE_COPILOT_API_BASE_URL

RUN pnpm build

FROM nginx:1.27-alpine

ENV API_PROXY_TARGET=http://api-go:8080
COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=builder /app/dist /usr/share/nginx/html
