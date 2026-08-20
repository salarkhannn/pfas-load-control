FROM node:20-alpine AS web-build

WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
ENV VITE_API_URL=""
RUN pnpm build

FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -tags netgo -ldflags='-s -w' -o /out/server ./cmd/server

FROM debian:bookworm-20260713-slim@sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates poppler-utils tesseract-ocr tesseract-ocr-eng \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 65532 app \
    && useradd --system --uid 65532 --gid app --home-dir /nonexistent --shell /usr/sbin/nologin app

COPY --from=build --chown=app:app /out/server /server
COPY --from=web-build --chown=app:app /src/web/dist /web
ENV WEB_STATIC_DIR=/web
USER app
EXPOSE 8080
ENTRYPOINT ["/server"]
