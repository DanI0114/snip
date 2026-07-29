# Build stage
FROM golang:1.26.5 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

# Runtime stage
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/server /app/server

COPY web /app/web
COPY migration /app/migration

EXPOSE 8080

CMD ["/app/server"]