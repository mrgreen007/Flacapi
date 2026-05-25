# FLAC API HTTP Service

A lightweight, robust Go-based HTTP wrapper for the **SpotiFLAC** download engine (`go_backend`). This service exposes endpoints mirroring the Android Gobindings, making the powerful FLAC/ALAC download logic accessible to web frontends, scripts, or external clients.

---

## 🚀 Features

- **Standard HTTP endpoints** translating Go/JSON payloads.
- **Asynchronous Background Pipeline**: Translates blocking download requests into non-blocking async processes, polling for progress, and serving finished tracks directly via HTTP.
- **Atomic Containment (Secure Staging)**: Isolates incoming data bytes in an isolated system temporary staging reservoir (`os.TempDir()/spotiflac_staging/`) during download, shielding active file locks and preventing concurrent request collisions.
- **HTTP Delivery & Auto-Cleanup**: Serves final audio file packages through structured HTTP binary streams, executing immediate disk reclamation sweeps upon client download success.
- **Ultra-Secure Sandbox Protection**: Dynamically clones extension runtime artifacts into the host system's isolated `os.TempDir()`. This shields you from Docker permission bottlenecks and ensures your git directory remains absolutely sterile.
- **Zero-Maintenance Dynamic Synchronizers**: Translucently interrogates the community extension registry at boot, hot-patching broken runtime tokens and refreshing delivery mirrors automatically.
- **Atomic Quality Sentinel**: Performs byte-level container inspection post-download; intercepting and automatically purging hidden codec downgrades before fallback cascades instantly engage.
- **Smart Conversions & Tagging**: Injects ID3v2/Vorbis tag blocks natively and optionally normalizes disparate lossless sources (like ALAC) into pure FLAC formats via high-speed processing pipelines.
- **Advanced Tracking Pipeline**: Implicit handle injection guarantees real-time speed, percentage, and status polling availability globally without manual client instrumentation.
- **Automated Retention Sweeper**: Background ticker dynamically sweeps expired completed/failed downloads after configurable periods.
- **Full Cross-Platform Compatibility**: Pure, isolated runtime engineering optimized natively for Windows, Mac, and Linux (Container-ready).

---

## 🛠 Prerequisites

- **Go**: Version `1.25` or higher (if compiling natively).
- **FFmpeg / FFprobe**: Required system environment PATH availability for atomic tagging and codec inspection.
- **Docker**: Fully integrated (includes internal FFmpeg distribution automatically).

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

> [!WARNING]
> **Read-Only Directories**: The `extensions` and `internal/go_backend` directories are managed by upstream remote repositories. **Do not modify files inside these folders manually.** Any local changes will be overwritten during sync updates.


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

A quick overview of active endpoints is provided below. For request payload structures, field-by-field definitions, response schemas, and curl/PowerShell client integration examples, please refer to the comprehensive **[API Reference Documentation (API.md)](./API.md)**.

### 🏥 System
- **`GET /health`** — Quick server health check. Returns local status and upstream dependency health (checking `api.zarz.moe`).

### 📥 Downloads & Progress
- **`POST /api/v1/download/strategy`** — Search and download audio asynchronously. Returns tracking `itemId` immediately.
- **`GET /api/v1/download/progress?itemId=<itemId>`** — Poll status, progress percentage, and error tracking for an item.
- **`GET /api/v1/download/progress/delta?since=<seq>`** — Retrieve delta updates since sequence number `<seq>`.
- **`GET /api/v1/download/file?itemId=<itemId>`** — Stream the completed audio file binary. Automatically deletes the file from disk upon successful retrieval.

### 🔄 Item Lifecycle Progress
- **`POST /api/v1/download/item/init`** — Initialize tracking for an item. `{ "itemId": "..." }`.
- **`POST /api/v1/download/item/finish`** — Mark an item as finished. `{ "itemId": "..." }`.
- **`POST /api/v1/download/item/clear`** — Clear progress for an item. `{ "itemId": "..." }`.
- **`POST /api/v1/download/item/cancel`** — Cancel an ongoing item download. `{ "itemId": "..." }`.

### 🎵 Metadata & Lyrics
- **`POST /api/v1/lyrics/get`** — Retrieve lyrics (LRC) for a track. `{ "spotifyId": "...", "trackName": "...", "artistName": "...", "durationMs": ... }`.


---

## 🔒 Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `FLACAPI_CONVERSION_STRATEGY` | Set to `FORCE_FLAC` to automatically convert all lossless deliveries to `.flac` format. | `ORIGINAL` |
| `FLACAPI_AUTO_UPDATE_EXTENSIONS` | Toggle automated mirror synchronization on boot cycles. | `true` |
| `FLACAPI_PROVIDER_PRIORITY` | Override fallback precedence chain (e.g., `apple-music,tidal-web`). | (System Default) |
| `FLACAPI_EXTENSION_STORE_URL` | Supply a manual extension store URL/File registry overriding community defaults. | (Public Repo) |
| `FLACAPI_APPLE_PROXY_KEY` | Authorization key enabling premium Apple Music provider proxy pipelines. | (Empty) |
| `FLACAPI_TIDAL_MIRROR_URL` | Private custom endpoint string overriding standard Tidal web extraction vectors. | (Empty) |
| `FLACAPI_TIDAL_TOKEN` | Dedicated scraper token injected implicitly into active Tidal payload requests. | (Empty) |
| `FLACAPI_RETENTION_HOURS` | Number of hours completed/failed download tasks & files are retained before cleanup sweeps. | `2` |

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

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
