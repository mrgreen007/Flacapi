# FLAC API HTTP Service

A lightweight, robust Go-based HTTP wrapper for the **SpotiFLAC** download engine (`go_backend`). This service exposes endpoints mirroring the Android Gobindings, making the powerful FLAC/ALAC download logic accessible to web frontends, scripts, or external clients.

---

## 🚀 Features

- **Standard HTTP endpoints** translating Go/JSON payloads.
- **Pristine Submodule Protection**: Automatically copies extension `.spotiflac-ext` packages from the clean git submodule into a git-ignored `./data/extensions_run` directory at startup. This guarantees your `extensions` and `go_backend` submodules remain **100% clean and unmodified** with no untracked extraction directories inside them!
- **Automatic Extension Enablement**: All 9 SpotiFLAC extensions are programmatically loaded and enabled automatically on server startup.
- **Consistent Output Directory Interceptor**: Intercepts `output_dir` in download payloads and resolves relative paths (like `./data/output`) to absolute paths. This ensures that **whatever extension or provider is used, the final audio file is always saved to the exact same folder**.
- **Security Check**: Safe path resolution with path-traversal protection.
- **Cross-Platform Support**: Works natively on Windows, macOS, and Linux (including Docker).

---

## 🛠 Prerequisites

- **Go**: Version `1.25` or higher (if running natively).
- **Docker**: Optional, for containerized environments.

---

## 🏃 Getting Started

### Option 1: Native Go (Local Dev)

1. **Build the binary**:
   ```powershell
   go build -o flacapi ./cmd/server
   ```
2. **Run the server**:
   ```powershell
   .\flacapi
   ```
   *Or run instantly:*
   ```powershell
   go run ./cmd/server
   ```
   
   The server starts on port `8080`.

### Option 2: Docker / Docker Compose

To spin up the service in a containerized environment (with persistence for downloaded data):
```bash
docker compose up --build
```

---

## 🔄 Syncing Submodules & Engine

This project uses a vendored copy of the Go backend (`internal/go_backend`) and an `extensions` submodule. Keep them updated using the sync scripts.

### On Windows (PowerShell):
```powershell
# Sync Extensions
.\scripts\sync-extensions.ps1

# Sync Go Backend (from SpotiFLAC-Mobile)
.\scripts\sync-go-backend.ps1
```

### On Linux / WSL / Git Bash:
```bash
# Sync Extensions
sh ./scripts/sync-extensions.sh

# Sync Go Backend
sh ./scripts/sync-go-backend.sh
```

---

## 📋 API Endpoints

A quick overview of active endpoints is provided below. For request payload structures, field-by-field definitions, response schemas, and curl/PowerShell client integration examples, please refer to the comprehensive [API Usage Documentation (API_USAGE.md)](file:///c:/Users/sabuj/Workspace/Projects/Sabuj.in/Flacapi/API_USAGE.md).

### 🏥 System
- **`GET /health`** — Quick server health check. Returns `{"status":"ok"}`.

### 📥 Downloads & Progress
- **`POST /api/v1/download/strategy`** — Search and download audio using a specific strategy. Accepts `{ "track_name": "...", "artist_name": "...", "output_dir": "..." }`.
- **`GET /api/v1/download/progress`** — Get individual download progress.
- **`GET /api/v1/download/progress/all`** — Retrieve progress list of all active downloads.
- **`GET /api/v1/download/progress/delta?since=<seq>`** — Retrieve delta updates since sequence number `<seq>`.

### 🔄 Item Lifecycle Progress
- **`POST /api/v1/download/item/init`** — Initialize tracking for an item. `{ "itemId": "..." }`.
- **`POST /api/v1/download/item/finish`** — Mark an item as finished. `{ "itemId": "..." }`.
- **`POST /api/v1/download/item/clear`** — Clear progress for an item. `{ "itemId": "..." }`.
- **`POST /api/v1/download/item/cancel`** — Cancel an ongoing item download. `{ "itemId": "..." }`.

### 🎵 Metadata & Lyrics
- **`POST /api/v1/metadata/read`** — Read file metadata. `{ "filePath": "..." }`.
- **`POST /api/v1/metadata/edit`** — Update file metadata. `{ "filePath": "...", "metadataJSON": ... }`.
- **`POST /api/v1/metadata/cover`** — Download cover art. `{ "coverUrl": "...", "outputPath": "...", "maxQuality": true }`.
- **`POST /api/v1/lyrics/get`** — Retrieve lyrics (LRC) for a track. `{ "spotifyId": "...", "trackName": "...", "artistName": "...", "filePath": "...", "durationMs": ... }`.
- **`POST /api/v1/lyrics/embed`** — Embed lyrics into a music file. `{ "filePath": "...", "lyrics": "..." }`.

### ⚙️ Configuration
- **`POST /api/v1/config/download-dir`** — Set default download directory safely. `{ "path": "..." }`.

---

## 🔒 Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `FLACAPI_DATA_DIR` | Base directory for persistent app databases and caching. | `./data` |
| `FLACAPI_DOWNLOADS_DIR` | Clean output folder specifically for audio downloads (Highly Recommended for Docker). | (Empty, defaults to DataDir) |
| `FLACAPI_EXTENSIONS_DIR` | Local directory containing SpotiFLAC source extensions. | `./extensions` |

---

## 🧼 Linting & CI/CD

This repository uses **`golangci-lint`** to enforce clean code and catch potential bugs early. 

### GitHub Actions
The repository is fully configured with a **GitHub Actions CI Workflow** (`.github/workflows/lint-build-test.yml`) that automatically runs both the linting suite and testing suite on every push or pull request to the `main` branch.

### Local Linting
To run the linters locally:
1. **Install golangci-lint**: Follow the [official installation guide](https://golangci-lint.run/welcome/install/).
2. **Run the linter**:
   ```bash
   golangci-lint run
   ```
   *Note: This automatically picks up our `.golangci.yml` rules and safely ignores the vendored `internal/go_backend` directory.*
