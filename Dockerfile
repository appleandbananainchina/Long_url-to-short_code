# 第一阶段：构建二进制
FROM golang:1.26-alpine AS builder

WORKDIR /app

# 复制 go.mod 和 go.sum 并下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 复制全部源代码
COPY . .

# 编译：因为 main.go 在 cmd/ 下，所以指定路径
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/short-url-service ./cmd/main.go

# 第二阶段：运行镜像
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# 从构建阶段复制二进制文件
COPY --from=builder /out/short-url-service .

# 如果根目录有默认配置文件（config.yaml 或 config.json），可取消注释下面一行
# COPY config.yaml .

EXPOSE 8080

CMD ["./short-url-service"]