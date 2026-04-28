FROM golang:1.26.2 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w -linkmode external -extldflags '-static'" -o torii ./cmd/torii

FROM debian:stable-slim

COPY --from=builder /app/torii /torii

VOLUME /data

WORKDIR /data

EXPOSE 27000

ENTRYPOINT ["/torii"]
CMD ["--config", "/etc/torii/config.yaml", "--data-dir", "/data"]
