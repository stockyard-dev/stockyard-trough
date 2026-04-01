FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /bin/trough ./cmd/trough/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata curl

COPY --from=builder /bin/trough /usr/local/bin/trough

# Environment variables — override at runtime
# DATA_DIR should be backed by a persistent volume in production
ENV PORT="8790" \
    DATA_DIR="/data" \
    TROUGH_ADMIN_KEY="changeme" \
    TROUGH_LICENSE_KEY=""

EXPOSE 8790

# Healthcheck — adjust interval for your use case
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -sf http://localhost:8790/health || exit 1

ENTRYPOINT ["trough"]
