# Stage 1 — Rust scanner
# pnet on Linux uses AF_PACKET (kernel feature); no libpcap needed.
FROM rust:slim AS scanner-build
WORKDIR /build/scanner
COPY scanner/Cargo.toml scanner/Cargo.lock* ./
# Pre-fetch dependencies before copying source (cache layer)
RUN cargo fetch
COPY scanner/src ./src
RUN cargo build --release

# Stage 2 — Go controller
FROM golang:1.24-bookworm AS controller-build
WORKDIR /build/controller
COPY controller/go.mod controller/go.sum* ./
RUN go mod download
COPY controller/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /ding ./cmd/ding

# Stage 3 — Minimal runtime image
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=scanner-build  /build/scanner/target/release/scanner  /usr/local/bin/scanner
COPY --from=controller-build /ding                                  /usr/local/bin/ding

RUN mkdir -p /data
VOLUME ["/data"]

# Requires --network host + CAP_NET_RAW + CAP_NET_ADMIN (see docker-compose.yml)
ENTRYPOINT ["/usr/local/bin/ding"]
