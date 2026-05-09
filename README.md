# FLAC API HTTP Service

A lightweight, robust Go-based HTTP wrapper for the **SpotiFLAC** download engine (`go_backend`). This service exposes endpoints mirroring the Android Gobindings, making the powerful FLAC download logic accessible to web frontends, scripts, or external clients.

---

## 🚀 Features

- **Standard HTTP endpoints** translating Go/JSON payloads.
- **Submodule & Extension Management**: Built-in support to load community extensions.
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

All data payloads are JSON.

### 🏥 System
- **`GET /health`** — Quick server health check. Returns `{"status":"ok"}`.

### 📥 Downloads & Progress
- **`POST /api/v1/download/strategy`** — Download using a specific SpotiFLAC strategy. Accepts `{ "requestJSON": "..." }`.
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
| `FLACAPI_DATA_DIR` | Root directory for downloads and app files. | `./data` |
| `FLACAPI_EXTENSIONS_DIR` | Directory containing SpotiFLAC extensions. | `./extensions` |
