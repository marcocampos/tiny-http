FROM golang:1.24-alpine AS builder

ARG CGO_ENABLED=0

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN go build -o tiny-http ./cmd

FROM alpine:latest

RUN mkdir -p /files
COPY ./static/index.html /files/

WORKDIR /app

COPY --from=builder /app/tiny-http .

EXPOSE 8080

CMD ["./tiny-http", "-directory", "/files"]
