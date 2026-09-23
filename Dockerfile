FROM golang:1.26.5-alpine3.23 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY cmd ./cmd
COPY internal ./internal

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath \
    -ldflags="-s -w -X main.version=0.1.0" \
    -o /out/endpoint-watchdog ./cmd/watchdog

FROM alpine:3.24.2 AS runtime

RUN addgroup -S -g 10001 watchdog \
    && adduser -S -D -H -u 10001 -G watchdog watchdog

WORKDIR /app
COPY --from=build --chown=watchdog:watchdog /out/endpoint-watchdog /app/endpoint-watchdog
COPY --chown=watchdog:watchdog config.example.yaml /app/config.yaml

USER watchdog
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/endpoint-watchdog"]
CMD ["-config", "/app/config.yaml"]

