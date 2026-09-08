# 允许替换 Go 构建镜像
ARG GO_IMAGE=golang:1.25-alpine

# 允许替换最终运行镜像
ARG RUNTIME_IMAGE=alpine:3.22

# 第一阶段：编译 Go 程序
FROM ${GO_IMAGE} AS builder

# 设置源码目录
WORKDIR /src

# 先复制依赖清单，利用 Docker 缓存
COPY go.mod go.sum ./

# 下载 Go 依赖
RUN go mod download

# 复制后端源码
COPY . .

# 编译 Linux 可执行文件
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" \
    -o /out/keystonego ./cmd/server


# 第二阶段：只运行编译结果
FROM ${RUNTIME_IMAGE}

# 安装 HTTPS 证书和时区数据
RUN apk add --no-cache ca-certificates tzdata

# 创建非 root 用户
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# 设置运行目录
WORKDIR /app

# 复制编译好的后端程序
COPY --from=builder /out/keystonego ./keystonego

# Casbin 启动时需要这个非敏感配置
COPY config/rbac_model.conf ./config/rbac_model.conf

# 允许程序生成 backup_cache.yaml
RUN chown -R appuser:appgroup /app

# 使用非 root 用户运行
USER appuser

# 声明后端监听端口
EXPOSE 8080

# 启动后端
CMD ["./keystonego"]