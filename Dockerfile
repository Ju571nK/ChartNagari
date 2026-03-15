# ── Frontend build stage ─────────────────────────────────────────────
FROM node:20-alpine AS frontend-builder
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# ── Go build stage ───────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /chart-analyzer ./cmd/server

# ── Run stage ────────────────────────────────────────────────────────
FROM alpine:3.19
RUN apk --no-cache add tzdata ca-certificates
WORKDIR /app
COPY --from=builder /chart-analyzer .
COPY config/ ./config/
COPY --from=frontend-builder /web/dist ./web/dist/
RUN mkdir -p /app/data
EXPOSE 8080
ENTRYPOINT ["/app/chart-analyzer"]
