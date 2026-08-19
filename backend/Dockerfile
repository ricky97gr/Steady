# ---- 构建阶段 ----
FROM golang:1.26-alpine AS builder

WORKDIR /build

# 国内网络环境使用代理镜像
ENV GOPROXY=https://goproxy.cn,direct

# 先复制依赖清单，利用 Docker 层缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/backend ./cmd/server

# ---- 运行阶段 ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/backend .
COPY configs /app/configs

EXPOSE 8080
CMD ["/app/backend"]
