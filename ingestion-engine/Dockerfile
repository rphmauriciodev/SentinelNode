# Multi-stage build for SentinelNode Ingestion Engine
FROM golang:alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/ingestion-engine ./cmd/main.go

# Final minimal runtime image
FROM alpine:3.22

# Install ca-certificates and tzdata for TLS validation and accurate timestamps
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/ingestion-engine /app/ingestion-engine

# Default environment configuration for containerized deployment
ENV MQTT_BROKER="ssl://broker:8883" \
    MQTT_CLIENT_ID="ingestion-engine-01" \
    DB_HOST="db" \
    DB_PORT="5432" \
    DB_USER="sentinel_user" \
    DB_PASSWORD="sentinel_password" \
    DB_NAME="sentinel_db" \
    DB_SSLMODE="disable" \
    MEDIA_GATEWAY_URL="http://media-gateway:8080/record" \
    CERTS_DIR="/certs" \
    PIR_CAMERA_MAPPING="esp32-pir-01:esp32-cam-01"

ENTRYPOINT ["/app/ingestion-engine"]
