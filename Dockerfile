# FogSense device runtime: one container = one simulated device.
# Multi-stage build producing a lightweight static binary (< 18 MB total image).

# Stage 1: Build static binary
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum* ./
COPY main.go ./
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /fogsense-device-runtime . && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /fogsense-mock-gateway ./cmd/mock-gateway

# Stage 2: Minimal runtime image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

COPY --from=builder /fogsense-device-runtime /usr/local/bin/fogsense-device-runtime
COPY --from=builder /fogsense-mock-gateway /usr/local/bin/fogsense-mock-gateway

USER app
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT:-8080}/health" >/dev/null || exit 1

ENTRYPOINT ["fogsense-device-runtime"]
