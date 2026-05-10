# ---- Build stage ----
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
WORKDIR /app

# Install git and build dependencies
RUN apk add --no-cache git build-base

# Copy go module files and download dependencies
COPY go.mod go.sum ./
COPY internal/go_backend ./internal/go_backend
RUN go mod download

# Copy source files
COPY . .

# Build the server binary
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o bin/flacapi ./cmd/server


# ---- Runtime stage ----
FROM alpine:3.20

# Install runtime dependencies (ffmpeg is required for ALAC to FLAC transcode and cover art embedding)
RUN apk add --no-cache ffmpeg

# Create a non-root group and user
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app

# Copy executable and extensions
COPY --from=builder /app/bin/flacapi ./flacapi
COPY --from=builder /app/extensions ./extensions

# Create default directories and adjust permissions
RUN mkdir -p /data /downloads /extensions && chown -R app:app /data /downloads /extensions /app

USER app
EXPOSE 8080

ENV FLACAPI_DATA_DIR=/data
ENV FLACAPI_DOWNLOADS_DIR=/downloads
ENV FLACAPI_EXTENSIONS_DIR=/app/extensions
ENV FLACAPI_CONVERSION_STRATEGY=ORIGINAL

ENTRYPOINT ["./flacapi"]
