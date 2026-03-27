# ── Stage 1: Build ──
FROM golang:1.26.1-alpine AS builder

WORKDIR /app

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
# CGO_ENABLED=0 — uses pure-Go SQLite driver, no gcc needed
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=$(cat VERSION 2>/dev/null || echo dev)" \
    -o vyanawatch .

# ── Stage 2: Final (minimal image) ──
FROM alpine:3.20

LABEL org.opencontainers.image.title="VyanaWatch" \
      org.opencontainers.image.description="Lightweight self-hosted uptime monitor" \
      org.opencontainers.image.source="https://github.com/vyanawatch/vyanawatch"

RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S vyanawatch && adduser -S vyanawatch -G vyanawatch

WORKDIR /app

COPY --from=builder /app/vyanawatch .
COPY --from=builder /app/config.yaml.example ./config.yaml.example

RUN mkdir -p /app/data && chown -R vyanawatch:vyanawatch /app

USER vyanawatch

EXPOSE 8080

VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["./vyanawatch"]
CMD ["--config", "/app/config.yaml"]
