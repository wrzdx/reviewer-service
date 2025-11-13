# syntax=docker/dockerfile:1

FROM golang:1.25 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o reviewer-service ./cmd

FROM debian:bookworm-slim

WORKDIR /app
COPY --from=builder /app/reviewer-service .
COPY .env .

EXPOSE 8080

CMD ["./reviewer-service"]
