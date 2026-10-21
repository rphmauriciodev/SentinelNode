# Multi-stage build for SentinelNode Media Gateway
FROM golang:alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/media-gateway .

# Final minimal runtime image
FROM alpine:3.22

# Install ffmpeg for MP4 stream encoding and recording, ca-certificates and tzdata
RUN apk add --no-cache ffmpeg ca-certificates tzdata wget

WORKDIR /app

# Ensure recordings directory exists
RUN mkdir -p /app/recordings

# Copy binary from builder
COPY --from=builder /app/media-gateway /app/media-gateway

ENV PORT=8080 \
    RECORDINGS_DIR=/app/recordings \
    DEFAULT_DEVICE_ID=esp32-cam-01 \
    TOLERANCE_SECONDS=30

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8080/status || exit 1

ENTRYPOINT ["/app/media-gateway"]
