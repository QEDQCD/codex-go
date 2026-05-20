FROM node:24-bullseye AS web
WORKDIR /src
COPY . .
RUN cd web && npm ci && npm run build

FROM golang:1.25 AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/cmd/codex-go/web-dist ./cmd/codex-go/web-dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/codex-go ./cmd/codex-go/

FROM node:24-bullseye-slim AS runtime
WORKDIR /app
ENV HOME=/root
RUN npm install -g @openai/codex@latest --include=optional \
    && codex --version \
    && (timeout 2 codex app-server --listen stdio:// >/tmp/codex-appserver.out 2>/tmp/codex-appserver.err; \
        code=$?; \
        if grep -q "Missing optional dependency" /tmp/codex-appserver.err; then cat /tmp/codex-appserver.err; exit 1; fi; \
        if [ "$code" != "0" ] && [ "$code" != "124" ]; then cat /tmp/codex-appserver.err; exit "$code"; fi)
COPY --from=build /out/codex-go /usr/local/bin/codex-go
ENTRYPOINT ["/usr/local/bin/codex-go"]
