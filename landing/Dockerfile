FROM golang:1.27-alpine AS builder

WORKDIR /app

# 依存関係だけ先にコピーしてキャッシュを効かせる
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /landing ./cmd/landing

FROM alpine:3.21

# tsnet がTailscaleのコントロールサーバーにHTTPS接続するのに必要
RUN apk add --no-cache ca-certificates

COPY --from=builder /landing /usr/local/bin/landing

ENTRYPOINT ["/usr/local/bin/landing"]
