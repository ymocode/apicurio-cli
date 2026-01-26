# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${VERSION}" \
    -o apicurio-client ./cmd/apicurio-client

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/apicurio-client /app/apicurio-client

# Create non-root user
RUN addgroup -g 1000 apicurio && \
    adduser -D -u 1000 -G apicurio apicurio && \
    chown -R apicurio:apicurio /app

USER apicurio

ENTRYPOINT ["/app/apicurio-client"]
CMD ["--help"]
