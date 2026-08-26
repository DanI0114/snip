# Build stage
FROM golang:1.26.5 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN echo "=== CHECKING FRONTEND ===" && \
    grep -n "SNIP_BUILD_MARKER_20260826" /app/web/static/app.js

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates \
    && adduser -D -H appuser

WORKDIR /app

COPY --from=builder /app/server ./server
COPY --from=builder /app/web ./web

RUN echo "=== CHECKING RUNTIME FRONTEND ===" && \
    grep -n "SNIP_BUILD_MARKER_20260826" /app/web/static/app.js

USER appuser

EXPOSE 10000

CMD ["./server"]