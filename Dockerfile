# Build
FROM golang:1.26-alpine AS build
WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/llm-proxy ./cmd/server

# Runtime
FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata wget su-exec \
  && adduser -D -H -u 10001 app \
  && mkdir -p /data \
  && chown app:app /data

COPY --from=build /out/llm-proxy /app/llm-proxy
COPY api/openapi.yaml /app/api/openapi.yaml
COPY docs /app/docs
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN sed -i 's/\r$//' /docker-entrypoint.sh && chmod +x /docker-entrypoint.sh

ENV ADDR=:8080 \
    OPENAPI_PATH=/app/api/openapi.yaml \
    RUNTIME_DATA_DIR=/data \
    DB_HOST=postgres \
    DB_PORT=5432 \
    DB_USER=llmproxy \
    DB_PASSWORD=llmproxy \
    DB_NAME=llmproxy \
    DB_SSLMODE=disable

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]
