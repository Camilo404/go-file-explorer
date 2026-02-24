FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/go-file-explorer ./cmd/server

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata ffmpeg su-exec

COPY --from=builder /bin/go-file-explorer /usr/local/bin/go-file-explorer
COPY docs /app/docs
COPY .env.example /app/.env.example
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /usr/local/bin/docker-entrypoint.sh

RUN addgroup -S explorer && adduser -S explorer -G explorer

RUN mkdir -p /data/.trash /data/.thumbnails /data/.chunks && \
    chown -R explorer:explorer /app /data

# Entrypoint runs as root so it can fix ownership of mounted volumes,
# then drops to the explorer user via su-exec.
ENV STORAGE_ROOT=/data
ENV TRASH_ROOT=/data/.trash
ENV THUMBNAIL_ROOT=/data/.thumbnails
ENV CHUNK_TEMP_DIR=/data/.chunks
ENV SERVER_PORT=8080

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
