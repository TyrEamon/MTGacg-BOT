FROM golang:1.21-alpine AS builder

WORKDIR /app

# 1. 先复制 go.mod (注意：这里我们暂时不复制 go.sum，因为你还没有)
COPY go.mod ./

# 2. 关键步骤：下载依赖并自动生成 go.sum
RUN go mod download && go mod tidy

# 3. 再复制剩下的代码
COPY . .

# 4. 编译
RUN go build -o bot main.go

# --- 运行阶段 ---
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/bot .

CMD ["./bot"]
