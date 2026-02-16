# Build stage
FROM golang:1.21 AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o hyperkaehler ./cmd/hyperkaehler

# Run stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /build/hyperkaehler .
COPY --from=builder /build/config.toml .
COPY --from=builder /build/.env .

RUN mkdir -p /app/data

CMD ["./hyperkaehler"]
