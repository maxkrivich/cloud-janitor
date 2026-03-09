# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
    -X github.com/maxkrivich/cloud-janitor/cmd.version=${VERSION} \
    -X github.com/maxkrivich/cloud-janitor/cmd.commit=${COMMIT} \
    -X github.com/maxkrivich/cloud-janitor/cmd.buildDate=${BUILD_DATE}" \
    -o cloud-janitor .

# Final stage - scratch for minimal size
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /app/cloud-janitor /cloud-janitor

ENTRYPOINT ["/cloud-janitor"]
