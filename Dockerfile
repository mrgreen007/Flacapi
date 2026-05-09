# ---- Build stage ----
FROM golang:1.23-alpine AS builder
WORKDIR /app

# Install git and build dependencies
RUN apk add --no-cache git build-base

# Copy go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source files
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/flacapi ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.20

# Create a non-root group and user
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app

# Copy executable and extensions
COPY --from=builder /app/bin/flacapi ./flacapi
COPY --from=builder /app/extensions ./extensions

# Create default directories and adjust permissions
RUN mkdir -p /data /extensions && chown -R app:app /data /extensions /app

USER app
EXPOSE 8080

ENV FLACAPI_DATA_DIR=/data
ENV FLACAPI_EXTENSIONS_DIR=/app/extensions

ENTRYPOINT ["./flacapi"]
