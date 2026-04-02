# 构建阶段
FROM golang:1.25-alpine AS builder

# Build argument to force cache invalidation
ARG BUILD_TIMESTAMP=default

WORKDIR /app

# 安装系统证书供后续 HTTPS 使用
RUN apk add --no-cache ca-certificates tzdata

# 复制依赖文件并下载
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码并编译（优化编译参数）
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o sora-video-server .

# 最终阶段 - 使用 scratch（最小化镜像）
FROM scratch

WORKDIR /app

# 复制 HTTPS 证书和时区数据进入纯净容器
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# 从构建阶段复制编译好的二进制文件
COPY --from=builder /app/sora-video-server .
# 将配置文件也复制过来，否则程序运行会报错找不到 json
COPY --from=builder /app/template_nsfw.json .

EXPOSE 8083

CMD ["./sora-video-server"]
