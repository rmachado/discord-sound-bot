FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o discord-sound-bot .

FROM alpine:3.20

RUN apk add --no-cache ffmpeg python3 ca-certificates && \
    wget -qO /usr/local/bin/yt-dlp https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp && \
    chmod a+x /usr/local/bin/yt-dlp

COPY --from=builder /app/discord-sound-bot /usr/local/bin/

WORKDIR /app
VOLUME ["/app/sounds"]

CMD ["discord-sound-bot"]
