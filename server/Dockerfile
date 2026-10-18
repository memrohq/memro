# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache dependencies separately from source
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/memro-api ./cmd/memro-api

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.22

RUN apk add --no-cache ca-certificates wget \
    && adduser -S -D -H -s /sbin/nologin memro \
    && mkdir -p /data/files && chown memro /data/files

COPY --from=builder /out/memro-api /usr/local/bin/memro-api

USER memro
ENV PORT=8081 FILE_STORAGE_PATH=/data/files
EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8081/health >/dev/null || exit 1

CMD ["memro-api"]
