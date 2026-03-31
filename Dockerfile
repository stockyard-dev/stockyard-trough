FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /bin/trough ./cmd/trough/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/trough /usr/local/bin/trough
ENV PORT=8790 \
    DATA_DIR=/data \
    TROUGH_ADMIN_KEY=""
EXPOSE 8790
ENTRYPOINT ["trough"]
