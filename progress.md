# Project Progress: FLAC API HTTP Service

## ✅ Completed
- [x] **Repository Initialization**: Git repository set up with `go_backend` and `extensions` as submodules/vendored directories.
- [x] **Dependency Management**: `go.mod` initialized and project structure defined.
- [x] **Vendoring Scripts**: `scripts/sync-go-backend.sh` and `scripts/sync-extensions.sh` are in place to keep the engine and extensions updated.
- [x] **Server Skeleton**: Basic HTTP server implemented in `cmd/server/main.go` with:
    - `chi` router for request handling.
    - CORS middleware for frontend compatibility.
    - Graceful shutdown logic.
    - Health check endpoint (`/health`).
- [x] **Core API Logic**: Created `internal/api/handlers.go` to bridge HTTP requests to the Go backend.

## 🚧 In Progress
- [ ] **Download by Strategy Endpoint**: Mapping `/api/v1/download/strategy` to the `DownloadByStrategy` backend function. (Logic implemented, finalizing routing).

## 📅 Next Steps
- [ ] **Item Lifecycle Endpoints**: Implement endpoints for initializing, finishing, clearing, and canceling download items.
- [ ] **Progress Tracking**: Implement `/api/v1/download/progress` for single and bulk progress updates.
- [ ] **Configuration API**: Add endpoints to manage the download directory and other global settings.
- [ ] **Metadata & Lyrics**: Implement endpoints for reading/editing audio metadata, fetching cover art, and downloading lyrics.
- [ ] **Extension Bootstrapping**: Ensure the extension system is initialized at server startup.
- [ ] **Security & Path Validation**: Implement strict checks for `FLACAPI_DATA_DIR` to prevent path traversal attacks.
- [ ] **Testing**: Develop unit tests for handlers and an integration test suite.
- [ ] **Deployment**: Create Dockerfile and docker-compose for containerized deployment.
- [ ] **CI/CD**: Set up GitHub Actions for automated builds and testing.
- [ ] **Documentation**: Complete the API reference and README for developers.
