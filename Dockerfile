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
ARG RUNSC_VERSION=latest
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
# CA certs are required for pulling images from HTTPS registries.
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=runsc-fetcher /tmp/runsc /usr/local/bin/runsc
COPY --from=builder /boxer /usr/local/bin/boxer

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/boxer"]
