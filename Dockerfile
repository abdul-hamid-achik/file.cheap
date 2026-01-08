FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN templ generate
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /worker ./cmd/worker

FROM alpine:latest AS api

RUN apk --no-cache add ca-certificates libwebp-tools poppler-utils
RUN adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /api .

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./api"]

FROM alpine:latest AS worker

RUN apk --no-cache add ca-certificates libwebp-tools poppler-utils
RUN adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /worker .

USER appuser

CMD ["./worker"]
