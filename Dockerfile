FROM golang:1.25-alpine AS deps
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download && go mod verify

FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates curl
RUN go install github.com/a-h/templ/cmd/templ@v0.3.977
# Install Tailwind CSS standalone CLI
RUN curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.3/tailwindcss-linux-arm64 && \
    chmod +x tailwindcss-linux-arm64 && \
    mv tailwindcss-linux-arm64 /usr/local/bin/tailwindcss
WORKDIR /app
COPY --from=deps /app/go.mod /app/go.sum ./
COPY . .
RUN templ generate
RUN tailwindcss -i static/css/input.css -o static/css/output.css --minify

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -tags=workers_image -o /worker-image ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /worker-video ./cmd/worker

FROM alpine:3.20 AS api
RUN apk --no-cache add ca-certificates
RUN adduser -D -g '' appuser
WORKDIR /app
COPY --from=builder /api .
COPY --from=builder /app/static ./static
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1
CMD ["./api"]

FROM alpine:3.20 AS worker-image
RUN apk --no-cache add ca-certificates libwebp-tools poppler-utils
RUN adduser -D -g '' appuser
WORKDIR /app
COPY --from=builder /worker-image ./worker
USER appuser
CMD ["./worker"]

FROM alpine:3.20 AS worker-video
RUN apk --no-cache add ca-certificates libwebp-tools poppler-utils ffmpeg
RUN adduser -D -g '' appuser
WORKDIR /app
COPY --from=builder /worker-video ./worker
USER appuser
CMD ["./worker"]
