# Project Plan: FLAC API HTTP Service

**Location**: `C:\Users\sabuj\Workspace\Projects\Sabuj.in\Flacapi`

**Goal**: Expose the existing `go_backend` (SpotiFLAC download engine) as a reusable HTTP/JSON micro‑service so that any client (Node.js/React, other languages, scripts) can request FLAC downloads, metadata extraction, cover art, lyrics, etc., without rewriting the Go logic.

**Approach**:
- Keep `go_backend` as an external dependency (git submodule) so upstream changes can be pulled.
- Keep the extension repository as a submodule (or subtree) to stay in sync with community extensions.
- Add a thin HTTP wrapper in Go that translates incoming HTTP requests to the same JSON request/response format used by the Android app.
- Provide Dockerfile and docker‑compose for easy local/dev deployment.
- Include basic CI (GitHub Actions) to build, test, and publish the image.
- Document the API endpoints mirroring the Android Gobindings.

---

## 1. Repository Setup

1. Initialize a new git repo (if not already):
   ```bash
   git init
   git remote add origin <your-repo-url>
   ```

2. Add `go_backend` as a submodule (tracking the upstream SpotiFLAC-Mobile go_backend):
   ```bash
   git submodule add https://github.com/zarzet/SpotiFLAC-Mobile.git go_backend
   # We only need the go_backend folder; later we can use sparse checkout or copy needed files.
   ```

   *Alternative*: use `go.mod` replace directive to point to a local path or a specific tag/commit.

3. Add the extension repo as a submodule (read‑only, to fetch extensions):
   ```bash
   git submodule add https://github.com/spotiflacapp/SpotiFLAC-Extension.git extensions
   ```

4. Create a Go module for the API service:
   ```bash
   go mod init sabuj.in/flacapi
   go get ./...
   ```

5. Ensure the Go compiler can find `go_backend`:
   - Option A: Copy the `go_backend` source into the module under `internal/go_backend` and keep it in sync via a script.
   - Option B: Use a `replace` directive in `go.mod`:
     ```
     replace github.com/zarz/spotiflac_android/go_backend => ./go_backend
     ```
   - Option C: Build `go_backend` as a separate binary and invoke it via subprocess (less preferred).

   For simplicity, we will **vendor** the `go_backend` source under `internal/go_backend` and sync with upstream using a script (`scripts/sync-go-backend.sh`).

---

## 2. HTTP Wrapper Design

### 2.1 Core Idea
- The Android app communicates with `go_backend` via JSON strings over a method channel (e.g., `downloadByStrategy`, `getAllDownloadProgress`, etc.).
- We will expose **matching HTTP endpoints** that accept the same JSON payload and return the same JSON string.

### 2.2 Endpoint Mapping (examples)

| Android Method          | HTTP Endpoint                | Method | Request Body               | Response |
|-------------------------|------------------------------|--------|----------------------------|----------|
| `downloadByStrategy`    | `/api/v1/download/strategy`  | POST   | `{ "requestJSON": "..." }` | JSON string |
| `getDownloadProgress`   | `/api/v1/download/progress`  | GET    | query: `itemId`            | JSON string |
| `getAllDownloadProgress`| `/api/v1/download/progress/all` | GET | — | JSON string |
| `initItemProgress`      | `/api/v1/download/item/init` | POST   | `{ "itemId": "..." }`      | (empty) |
| `finishItemProgress`    | `/api/v1/download/item/finish`| POST  | `{ "itemId": "..." }`      | (empty) |
| `clearItemProgress`     | `/api/v1/download/item/clear`| POST   | `{ "itemId": "..." }`      | (empty) |
| `cancelDownload`        | `/api/v1/download/item/cancel`| POST  | `{ "itemId": "..." }`      | (empty) |
| `setDownloadDirectory`  | `/api/v1/config/download-dir`| POST   | `{ "path": "..." }`        | (empty) |
| `readAudioMetadata…`    | `/api/v1/metadata/read`      | POST   | `{ "filePath": "...", "hint": "...", "coverCacheKey": "..." }` | JSON string |
| … (add others as needed) |                              |        |                            |          |

- All JSON responses from Go functions are passed through as `application/json` with the raw string (the Go functions already return a JSON‑encoded string).
- Errors are returned with appropriate HTTP status codes (400 for bad request, 500 for internal) and a JSON error object: `{"error":"…","message":"…"}`.

### 2.3 Implementation Steps

1. Create `cmd/server/main.go`:
   - Set up `net/http` router (use `github.com/go-chi/chi` or standard `http.ServeMux`).
   - Middleware for logging, CORS (allow `*` for dev, restrict later), and JSON decoding/encoding.
   - For each endpoint, decode incoming JSON (if any), call the corresponding `go_backend` function, write the result.

2. Reuse existing Go functions:
   - Import `github.com/zarz/spotiflac_android/go_backend` (via replace/vendoring).
   - Call functions like `DownloadByStrategy`, `GetAllDownloadProgress`, etc., directly.

3. Handle file‑system paths:
   - The service will receive paths from the client (e.g., output directory, file paths). We will **not** attempt to access Android SAF; instead we expect the caller to provide a regular filesystem path that the service can read/write.
   - Add a base `dataDir` configurable via env (`FLACAPI_DATA_DIR`) or flag; all relative paths are resolved against it for safety.

4. Extension loading:
   - At startup, scan the `extensions` submodule for `.zip` or `.json` extension manifests and load them via the existing extension system (`InitExtensionSystem`, `LoadExtensionsFromDir`).
   - Ensure the extensions have read access to the `extensions` folder.

5. Graceful shutdown:
   - Listen for `os.Signal` (SIGINT, SIGTERM) and call `http.Server.Shutdown`.

### 2.4 Code Layout (proposed)

```
/cmd
    /server
        main.go
/internal
    /go_backend   // vendored copy of go_backend source (or symlink via replace)
    /api          // HTTP handlers, middleware
/config
    config.go     // load env/flags
/scripts
    sync-go-backend.sh   // updates vendored go_backend from upstream
    sync-extensions.sh   // pulls extension submodule
/Dockerfile
/docker-compose.yml
/go.mod
/go.sum
/README.md
/PLAN.md   <-- this file
```

---

## 3. Dependency Management & Upstream Updates

- **go_backend**: We'll keep a vendored copy under `internal/go_backend` and provide a script to update it:
  ```bash
  #!/usr/bin/env bash
  set -e
  UPSTREAM=https://github.com/zarzet/SpotiFLAC-Mobile.git
  TMP=$(mktemp -d)
  git clone --depth 1 $UPSTREAM $TMP
  cp -r $TMP/go_backend/* internal/go_backend/
  rm -rf $TMP
  go mod tidy
  ```
  Commit the updated vendor after verification.

- **extensions**: Similar script to pull latest:
  ```bash
  #!/usr/bin/env bash
  set -e
  git submodule update --remote --merge extensions
  ```

- CI will run these scripts on a schedule (e.g., nightly) to ensure we stay current.

---

## 4. Dockerization

**Dockerfile (multi‑stage)**

```dockerfile
# ---- Build stage ----
FROM golang:1.23-alpine AS builder
WORKDIR /app
# Install git (needed for go modules that may fetch)
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/flacapi ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /app/bin/flacapi ./flacapi
# Copy default extensions (optional)
COPY extensions ./extensions
# Create a writable data directory for downloads/cache
RUN mkdir -p /data && chown app:app /data
USER app
EXPOSE 8080
ENV FLACAPI_DATA_DIR=/data
ENTRYPOINT ["./flacapi"]
```

**docker-compose.yml (dev)**

```yaml
version: "3.8"
services:
  flacapi:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data          # persistent storage for downloads
      - ./extensions:/app/extensions:ro  # keep extensions in sync
    environment:
      - FLACAPI_DATA_DIR=/data
    restart: unless-stopped
```

---

## 5. Testing Strategy

1. **Unit Tests** for HTTP handlers (using `httptest`).
2. **Integration Tests** that spin up the server (in‑process) and call endpoints with sample JSON requests (e.g., a mock extension that returns a dummy file).
3. **Contract Tests**: Ensure the JSON request/response format matches what the Android app expects. We can reuse existing Android test vectors if available.
4. **CI**: GitHub Actions workflow that:
   - Checks out repo with submodules.
   - Runs `go test ./...`.
   - Builds Docker image.
   - Pushes image to a registry (optional) on tag push.

---

## 6. Documentation

- **API Reference**: Auto‑generated markdown from Go comments (using `swag` or manual) listing each endpoint, request schema, response schema, and example curl.
- **README**: Quick start (`docker compose up`), how to call from Node.js (`fetch`/`axios`), how to update extensions/go_backend.
- **CHANGELOG**: Track changes to the wrapper.

---

## 7. Milestones & Tasks (Task IDs)

| ID | Subject                                   | Description |
|----|-------------------------------------------|-------------|
| 1  | Initialize repo & submodules              | Create git repo, add go_backend & extensions as submodules, set up go.mod |
| 2  | Vendoring script for go_backend           | Write `scripts/sync-go-backend.sh` to copy latest go_backend source |
| 3  | Vendoring script for extensions           | Write `scripts/sync-extensions.sh` to pull extension updates |
| 4  | Basic HTTP server skeleton                | `cmd/server/main.go` with chi router, logging, CORS |
| 5  | Implement downloadByStrategy endpoint     | Map `/api/v1/download/strategy` POST to `DownloadByStrategy` |
| 6  | Implement progress endpoints              | `/api/v1/download/progress` (single & all) |
| 7  | Implement item lifecycle endpoints        | init/finish/clear/cancel |
| 8  | Implement config endpoints                | set download dir, get current config |
| 9  | Implement metadata/cover/lyrics endpoints | as needed (metadata read, edit, cover download, lyrics fetch) |
| 10 | Extension loading at startup              | Scan `extensions` folder and init extension system |
| 11 | Add safety checks for file paths          | Resolve against `FLACAPI_DATA_DIR`, reject path traversal |
| 12 | Write unit tests for handlers             | Use `httptest` |
| 13 | Write integration test suite              | Spin up server, call endpoints with mock extension |
| 14 | Dockerfile & compose                      | Create multi‑stage Dockerfile and dev compose |
| 15 | CI workflow (GitHub Actions)              | Build, test, push image on tags |
| 16 | Documentation (README, API reference)    | Write usage docs and example Node/React snippets |
| 17 | Release v0.1.0                            | Tag and publish first version |

---

## 8. Future Enhancements

- gRPC version for lower‑latency, strongly‑typed communication.
- WebSocket endpoint for real‑time progress streaming.
- Auth token validation (API key or JWT) for multi‑tenant usage.
- Metrics endpoint (`/metrics`) for Prometheus.
- Automatic extension updates via a cron job that checks the extension repo for new releases.

---

**End of Plan**.  
Proceed with task creation (using TaskCreate tool) if you wish to track each milestone. The plan is version‑controlled here for reference and can be updated as the project evolves.
