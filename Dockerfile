# Build stage
FROM golang:1.26.5 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates \
    && adduser -D -H appuser

WORKDIR /app

COPY --from=builder /app/server ./server
COPY --from=builder /app/webfiles ./webfiles

USER appuser

EXPOSE 10000

CMD ["./server"]