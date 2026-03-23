# Stage 1: Build boxer
FROM golang:1.25 AS builder
WORKDIR /src
COPY packages/core/go.mod packages/core/go.sum ./
RUN go mod download
COPY packages/core/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /boxer .

# Stage 2: Runtime image
FROM debian:bookworm-slim

# CA certs are required for pulling images from HTTPS registries.
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
    && rm -rf /var/lib/apt/lists/*

# Install runsc (gVisor). Supports amd64 and arm64 hosts.
ARG RUNSC_VERSION=latest
RUN ARCH=$(dpkg --print-architecture | sed 's/amd64/x86_64/;s/arm64/aarch64/') && \
    curl -fsSL "https://storage.googleapis.com/gvisor/releases/release/${RUNSC_VERSION}/${ARCH}/runsc" \
        -o /usr/local/bin/runsc && \
    chmod 755 /usr/local/bin/runsc

COPY --from=builder /boxer /usr/local/bin/boxer

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/boxer"]
