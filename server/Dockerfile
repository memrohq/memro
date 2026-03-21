# ── Build stage ──────────────────────────────────────────────────────────────
FROM rust:1.83-slim AS builder

WORKDIR /usr/src/app

# Cache dependencies separately from source
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo 'fn main(){}' > src/main.rs \
    && cargo build --release \
    && rm -rf src

COPY . .
# Touch main.rs so cargo re-links (dependency cache hit)
RUN touch src/main.rs && cargo build --release

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# Non-root user
RUN useradd --system --no-create-home --shell /usr/sbin/nologin memro

COPY --from=builder /usr/src/app/target/release/ax_memro /usr/local/bin/ax_memro

USER memro

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8081/health || exit 1

CMD ["ax_memro"]
