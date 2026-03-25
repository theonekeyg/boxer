# Stage 1: Build boxer
FROM golang:1.25 AS builder
WORKDIR /src
COPY packages/core/go.mod packages/core/go.sum ./
RUN go mod download
COPY packages/core/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /boxer .

# Stage 2: Download and verify runsc (gVisor). Supports amd64 and arm64 hosts.
# curl is kept out of the final runtime image by doing the download here.
FROM debian:bookworm-slim AS runsc-fetcher
ARG RUNSC_VERSION=20260316.0
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
    && rm -rf /var/lib/apt/lists/*
RUN ARCH=$(dpkg --print-architecture | sed 's/amd64/x86_64/;s/arm64/aarch64/') && \
    BASE="https://storage.googleapis.com/gvisor/releases/release/${RUNSC_VERSION}/${ARCH}" && \
    curl -fsSL "${BASE}/runsc"        -o /tmp/runsc && \
    curl -fsSL "${BASE}/runsc.sha512" -o /tmp/runsc.sha512 && \
    (cd /tmp && sha512sum -c runsc.sha512) && \
    chmod 755 /tmp/runsc

# Stage 3: Minimal runtime image — no curl, no build tools.
FROM debian:bookworm-slim
# ca-certificates: required for pulling images from HTTPS registries.
# wget: used by the HEALTHCHECK probe below.
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        wget \
    && rm -rf /var/lib/apt/lists/*
COPY --from=runsc-fetcher /tmp/runsc /usr/local/bin/runsc
COPY --from=builder /boxer /usr/local/bin/boxer
# Default config for containerised deployments:
#   ignore_cgroups: Docker already owns the outer cgroup hierarchy; nested
#                   cgroup creation fails without explicit host delegation.
#   dns_servers:    Explicit resolvers instead of reading /etc/resolv.conf,
#                   which reflects the host's stub resolver and is meaningless
#                   inside an isolated network namespace.
# Users can override by mounting a config file and setting $BOXER_CONFIG.
COPY docker/config.json /etc/boxer/config.json
ENV BOXER_CONFIG=/etc/boxer/config.json

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/boxer"]
