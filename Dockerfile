# Build stage
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

ARG TARGETARCH

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations
RUN GOARCH=${TARGETARCH} CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o k8s-image-updater

# Final stage
FROM scratch

# Copy SSL certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/k8s-image-updater /

ENV GIN_MODE=release

EXPOSE 8080
ENTRYPOINT ["/k8s-image-updater"] 