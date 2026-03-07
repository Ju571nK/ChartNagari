# ── Build stage ───────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 의존성 캐싱 (소스 변경 시 재빌드 최소화)
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /chart-analyzer ./cmd/server

# ── Run stage ─────────────────────────────────────────────────────────
FROM alpine:3.19

# 타임존 + CA 인증서
RUN apk --no-cache add tzdata ca-certificates

WORKDIR /app

# 바이너리 복사
COPY --from=builder /chart-analyzer .

# 설정 파일 복사
COPY config/ ./config/

# 데이터 폴더 생성 (SQLite 저장 경로)
RUN mkdir -p /app/data

EXPOSE 8080

ENTRYPOINT ["/app/chart-analyzer"]
