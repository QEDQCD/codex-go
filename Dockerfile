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
COPY --from=web /src/cmd/cc-go/web-dist ./cmd/cc-go/web-dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/codex-go ./cmd/cc-go/

FROM node:24-bullseye-slim AS runtime
WORKDIR /app
ENV HOME=/root
RUN npm install -g @openai/codex@latest
COPY --from=build /out/codex-go /usr/local/bin/codex-go
ENTRYPOINT ["/usr/local/bin/codex-go"]
