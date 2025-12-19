FROM golang:1.21-alpine AS builder

WORKDIR /app

# 1. 先把所有文件都复制进去 (简单粗暴，最有效)
COPY . .

# 2. 现在有了代码，go mod tidy 就能扫描到依赖并生成 go.sum 了
RUN go mod tidy

# 3. 下载依赖 (其实上一步 tidy 已经下载了，但这步可以确保缓存)
RUN go mod download

# 4. 编译
RUN go build -o bot main.go

# --- 运行阶段 ---
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/bot .

CMD ["./bot"]
