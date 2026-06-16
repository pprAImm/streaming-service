FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o streaming-server ./cmd/server

FROM alpine:3.18

RUN apk add --no-cache ffmpeg ca-certificates

WORKDIR /app

COPY --from=builder /app/streaming-server .

RUN mkdir -p /app/videos /app/transcoded

EXPOSE 8082

CMD ["./streaming-server"]
