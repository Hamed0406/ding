# Stage 1 — Build React UI
FROM node:22-alpine AS ui-build
WORKDIR /build/ui
COPY ui/package*.json ./
RUN npm install
COPY ui/ ./
RUN npm run build

# Stage 2 — Build Rust scanner
# pnet on Linux uses AF_PACKET (kernel feature); no libpcap needed.
FROM rust:slim AS scanner-build
WORKDIR /build/scanner
COPY scanner/Cargo.toml scanner/Cargo.lock* ./
RUN cargo fetch
COPY scanner/src ./src
RUN cargo build --release

# Stage 3 — Build Go controller
# The ui/dist output is copied into internal/api/static/ before go build
# so that go:embed can bundle it into the binary.
FROM golang:1.25-bookworm AS controller-build
WORKDIR /build/controller
COPY controller/go.mod controller/go.sum* ./
RUN go mod download
COPY controller/ ./
COPY --from=ui-build /build/ui/dist/ ./internal/api/static/
RUN CGO_ENABLED=0 GOOS=linux go build -o /ding ./cmd/ding

# Stage 4 — Minimal runtime image
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=scanner-build  /build/scanner/target/release/scanner  /usr/local/bin/scanner
COPY --from=controller-build /ding                                  /usr/local/bin/ding

RUN mkdir -p /data
VOLUME ["/data"]

# Requires --network host + CAP_NET_RAW + CAP_NET_ADMIN (see docker-compose.yml)
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ding"]
