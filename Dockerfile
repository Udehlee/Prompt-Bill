FROM golang:1.27-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /tracker ./cmd/tracker


FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S tracker && adduser -S tracker -G tracker

WORKDIR /app
COPY --from=builder /tracker /app/tracker
USER tracker

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/tracker"]