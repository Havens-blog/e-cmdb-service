# 多阶段构建 Dockerfile
# 构建方式: docker build -f deploy/Dockerfile -t e-cmdb-service:0.1.0 .

# 构建阶段
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# 复制 go mod 文件并下载依赖（含外部模块 e-cloudx-sdk / e-common-go）
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码（本服务采用手动依赖注入，无需 wire 代码生成）
COPY . .

# 构建应用程序
ARG VERSION=dev
ARG BUILD_TIME
ARG COMMIT_HASH

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION}" \
    -a -installsuffix cgo \
    -o e-cmdb-service .

# 运行阶段
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata wget

RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/e-cmdb-service .
COPY --from=builder /app/config ./config

RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE 8002

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8002/api/v1/cmdb/health || exit 1

CMD ["./e-cmdb-service", "start"]
