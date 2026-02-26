# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git for go mod (no CGO needed)
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (cache bust: 2026-01-16-v2)
COPY . .

# Build the application with optimizations (CGO disabled for faster build)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o server .

# Runtime stage - use alpine:3.21 to match golang:1.24-alpine base
FROM alpine:3.21

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl

# Copy binary from builder
COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations

# Create non-root user
RUN adduser -D -g '' appuser && chown -R appuser:appuser /app
USER appuser

# Expose port
EXPOSE 8080

# Health check with curl (more reliable than wget)
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run the application
CMD ["./server", "--mode=all"]
