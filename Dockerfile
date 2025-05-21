# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tiny-http ./cmd/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS support
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/tiny-http .

# Create directory for static files
RUN mkdir -p /static && chown -R appuser:appgroup /static

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Set default environment variables
ENV PORT=8080
ENV HOSTNAME=0.0.0.0
ENV LOG_LEVEL=info

# Run the application
ENTRYPOINT ["./tiny-http"]
CMD ["-directory", "/static", "-port", "8080", "-hostname", "0.0.0.0", "-log-level", "info"]
