FROM golang:1.24-alpine AS builder

ARG CGO_ENABLED=0

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN go build -o tiny-http ./cmd

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/tiny-http .

# Expose the default port
EXPOSE 8080

# Command to run the server
CMD ["./tiny-http"]