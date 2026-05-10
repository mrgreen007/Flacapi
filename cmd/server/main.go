package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/handlers"

	go_backend "github.com/zarz/spotiflac_android/go_backend"
	"sabuj.in/flacapi/internal/api"
)

func loadDotEnv() {
	content, err := os.ReadFile(".env")
	if err != nil {
		return // Quiet if not found
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			// Preserve potential quotes around values
			val = strings.Trim(val, `"'`)
			if os.Getenv(key) == "" { // Don't override already set shell vars
				os.Setenv(key, val)
			}
		}
	}
}

func main() {
	// First load local definitions from file if running outside runtime containers
	loadDotEnv()

	// Retrieve paths from environment or use defaults
	dataDir := os.Getenv("FLACAPI_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	extensionsDir := os.Getenv("FLACAPI_EXTENSIONS_DIR")
	if extensionsDir == "" {
		extensionsDir = "./extensions"
	}
	downloadsDir := os.Getenv("FLACAPI_DOWNLOADS_DIR")

	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		log.Fatalf("Invalid data directory path %s: %v", dataDir, err)
	}
	absExtDir, err := filepath.Abs(extensionsDir)
	if err != nil {
		log.Fatalf("Invalid extensions directory path %s: %v", extensionsDir, err)
	}

	var absDownloadsDir string
	if downloadsDir != "" {
		absDownloadsDir, err = filepath.Abs(downloadsDir)
		if err != nil {
			log.Fatalf("Invalid downloads directory path %s: %v", downloadsDir, err)
		}
	}

	// Ensure directories exist
	if err := os.MkdirAll(absDataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory %s: %v", absDataDir, err)
	}
	if err := os.MkdirAll(absExtDir, 0755); err != nil {
		log.Fatalf("Failed to create extensions directory %s: %v", absExtDir, err)
	}
	if absDownloadsDir != "" {
		if err := os.MkdirAll(absDownloadsDir, 0755); err != nil {
			log.Fatalf("Failed to create downloads directory %s: %v", absDownloadsDir, err)
		}
	}

	// Determine active source extensions directory
	srcExtDir := absExtDir
	if _, err := os.Stat(filepath.Join(absExtDir, "extensions")); err == nil {
		srcExtDir = filepath.Join(absExtDir, "extensions")
	}

	// Point run-time extensions to the system temporary pool to prevent volume mount permission issues in containers
	runExtDir := filepath.Join(os.TempDir(), "spotiflac_extensions_run")
	_ = os.RemoveAll(runExtDir)
	if err := os.MkdirAll(runExtDir, 0755); err != nil {
		log.Fatalf("Failed to create run-time extensions directory %s: %v", runExtDir, err)
	}

	// Flush any residual garbage accumulated from abrupt crashes in transaction staging pools
	_ = os.RemoveAll(filepath.Join(os.TempDir(), "spotiflac_staging"))

	// Copy .spotiflac-ext packages to the run-time directory
	if err := copyExtensions(srcExtDir, runExtDir); err != nil {
		log.Printf("Warning: Failed to copy extension packages from %s to %s: %v", srcExtDir, runExtDir, err)
	}

	// Configure handler base directory
	api.DataDir = absDataDir

	// Global Branding: Explicitly declare app context versioning to assure compatibility with remote API user-agent gating
	go_backend.SetAppVersion("1.2.2")

	// Initialize backend default download folder and setup path safety boundaries
	defaultDownloadPath := absDataDir
	if absDownloadsDir != "" {
		defaultDownloadPath = absDownloadsDir
		// Allow and record independent download mount so SafePath grants access
		api.AdditionalAllowedDirs = append(api.AdditionalAllowedDirs, absDownloadsDir)
		go_backend.AllowDownloadDir(absDownloadsDir)
	}

	api.DefaultDownloadDir = defaultDownloadPath

	// Map custom conversion strategy for container finalization behaviors
	if convStrat := os.Getenv("FLACAPI_CONVERSION_STRATEGY"); convStrat != "" {
		api.ConversionStrategy = strings.TrimSpace(convStrat)
	}

	if err := go_backend.SetDownloadDirectory(defaultDownloadPath); err != nil {
		log.Printf("Warning: Failed to set default download directory: %v", err)
	}
	go_backend.AllowDownloadDir(absDataDir)

	// Inject optional dynamic runtime configurations derived from environment scope
	bootstrapExtensionConfigs(absDataDir)

	// Bootstrap extension system using the clean run-time directory
	if err := go_backend.InitExtensionSystem(runExtDir, absDataDir); err != nil {
		log.Printf("Warning: Failed to initialize extension system: %v", err)
	} else {
		log.Println("Extension system initialized successfully")
		loaded, err := go_backend.LoadExtensionsFromDir(runExtDir)
		if err != nil {
			log.Printf("Warning: Failed to load extensions from %s: %v", runExtDir, err)
		} else {
			log.Printf("Loaded extensions: %s", loaded)
			for _, extID := range []string{"amazon", "apple-music", "deezer", "pandora", "qobuz-web", "soundcloud", "spotify-web", "tidal-web", "ytmusic-spotiflac"} {
				if err := go_backend.SetExtensionEnabledByID(extID, true); err != nil {
					log.Printf("Warning: Failed to enable extension %s: %v", extID, err)
				} else {
					log.Printf("Successfully enabled extension at startup: %s", extID)
				}
			}

			// Configure default fallback priorities to support automatic multi-provider lossless downloads
			defaultPriority := []string{"apple-music", "tidal-web", "qobuz-web", "deezer", "amazon", "ytmusic-spotiflac", "soundcloud", "pandora"}
			finalPriority := defaultPriority

			if customPriorityStr := os.Getenv("FLACAPI_PROVIDER_PRIORITY"); customPriorityStr != "" {
				var customParts []string
				for _, p := range strings.Split(customPriorityStr, ",") {
					if p = strings.TrimSpace(p); p != "" {
						customParts = append(customParts, p)
					}
				}
				if len(customParts) > 0 {
					finalPriority = customParts
					log.Printf("Custom provider priority applied from environment: %v", finalPriority)
				}
			}

			go_backend.SetProviderPriority(finalPriority)
			go_backend.SetExtensionFallbackProviderIDs(finalPriority)

			// Dynamic configuration: synchronize extensions with the latest available cloud mirrors and configurations on startup
			if os.Getenv("FLACAPI_AUTO_UPDATE_EXTENSIONS") != "false" {
				log.Println("Checking for updated extension payloads and refreshed mirror bundles from GitHub marketplace...")
				autoUpdateExtensions(absDataDir)
			}
		}
	}

	r := chi.NewRouter()

	// Basic middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS
	corsHandler := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Accept", "Content-Type", "Authorization", "X-Requested-With"}),
		handlers.AllowCredentials(),
	)

	// Health check endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// API Routing
	r.Route("/api/v1", func(r chi.Router) {
		// Custom Extension Search
		r.Post("/search", api.HandleCustomSearch)

		// Downloads & Progress
		r.Post("/download/strategy", api.HandleDownloadByStrategy)
		r.Get("/download/progress", api.HandleGetDownloadProgress)
		r.Get("/download/progress/all", api.HandleGetAllDownloadProgress)
		r.Get("/download/progress/delta", api.HandleGetAllDownloadProgressDelta)

		// Item Progress Lifecycle
		r.Post("/download/item/init", api.HandleInitItemProgress)
		r.Post("/download/item/finish", api.HandleFinishItemProgress)
		r.Post("/download/item/clear", api.HandleClearItemProgress)
		r.Post("/download/item/cancel", api.HandleCancelDownload)

		// Catalog & Availability
		r.Post("/catalog/availability", api.HandleCheckAvailability)
		r.Post("/catalog/availability/platform", api.HandleCheckAvailabilityByPlatform)
		r.Post("/catalog/resolve-id", api.HandleResolveID)
		r.Post("/catalog/metadata", api.HandleGetProviderMetadata)

		// Metadata
		r.Post("/metadata/read", api.HandleReadMetadata)
		r.Post("/metadata/edit", api.HandleEditMetadata)
		r.Post("/metadata/cover", api.HandleDownloadCover)
		r.Post("/metadata/extract-cover", api.HandleExtractCover)

		// Lyrics
		r.Post("/lyrics/get", api.HandleGetLyrics)
		r.Post("/lyrics/embed", api.HandleEmbedLyrics)

		// Deduplication
		r.Post("/download/duplicate/check", api.HandleCheckDuplicate)
		r.Post("/download/duplicate/check-batch", api.HandleCheckDuplicatesBatch)

		// Library & Cue Sheet
		r.Post("/library/parse-cue", api.HandleParseCueSheet)

		// Configuration
		r.Post("/config/download-dir", api.HandleSetDownloadDir)
	})

	// Wrap router with CORS
	handler := corsHandler(r)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  15 * time.Minute,
		WriteTimeout: 15 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Println("Starting FLAC API server on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Shutting down server...")

	// Create a context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown:%v", err)
	}

	log.Println("Server stopped")
}

func copyExtensions(srcDir, destDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".spotiflac-ext") {
			srcFile := filepath.Join(srcDir, entry.Name())
			destFile := filepath.Join(destDir, entry.Name())
			if err := copyFile(srcFile, destFile); err != nil {
				return err
			}
		}
	}
	return nil
}

// storeExtensionResponse defines the internal extension metadata received from the store registry
type storeExtensionResponse struct {
	ID               string `json:"id"`
	DisplayName      string `json:"display_name"`
	Version          string `json:"version"`
	IsInstalled      bool   `json:"is_installed"`
	HasUpdate        bool   `json:"has_update"`
	InstalledVersion string `json:"installed_version"`
}

// autoUpdateExtensions contacts the central community repository on launch to ensure absolute current delivery mirrors
func autoUpdateExtensions(absDataDir string) {
	// Ensure underlying store instance is registered with the persistent backend prior to polling
	_ = go_backend.InitExtensionStoreJSON(absDataDir)

	// Set the official centralized extension repository (Standard SpotiFLAC Mobile defaults)
	defaultRepo := "https://github.com/spotiflacapp/SpotiFLAC-Extension"
	if customRepo := os.Getenv("FLACAPI_EXTENSION_STORE_URL"); customRepo != "" {
		defaultRepo = customRepo
	}

	if err := go_backend.SetStoreRegistryURLJSON(defaultRepo); err != nil {
		log.Printf("[ExtensionSync] Failed to establish store reference: %v", err)
		return
	}

	// Force a live poll to calculate current state against registry delta
	resJSON, err := go_backend.GetStoreExtensionsJSON(true)
	if err != nil {
		log.Printf("[ExtensionSync] Network synchronization failed, deferring to cached packages: %v", err)
		return
	}

	var extensions []storeExtensionResponse
	if err := json.Unmarshal([]byte(resJSON), &extensions); err != nil {
		log.Printf("[ExtensionSync] Encountered invalid catalog format: %v", err)
		return
	}

	// Construct unique temporary directory in system temp pool for secure artifact extraction
	tempDir := filepath.Join(os.TempDir(), "spotiflac_extension_sync_tmp")
	_ = os.RemoveAll(tempDir) // Flush previous leftover buffers
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Printf("[ExtensionSync] Local staging access denied: %v", err)
		return
	}
	defer os.RemoveAll(tempDir)

	var upgradesPerformed int
	for _, ext := range extensions {
		if ext.IsInstalled && ext.HasUpdate {
			log.Printf("[ExtensionSync] Identified active patch for %s: [v%s -> v%s]", ext.DisplayName, ext.InstalledVersion, ext.Version)

			// Stream latest package from repository pool
			filePath, err := go_backend.DownloadStoreExtensionJSON(ext.ID, tempDir)
			if err != nil {
				log.Printf("[ExtensionSync] Transfer aborted for %s: %v", ext.DisplayName, err)
				continue
			}

			// Commit overwrite directly to live environment context
			if _, err := go_backend.UpgradeExtensionFromPath(filePath); err != nil {
				log.Printf("[ExtensionSync] Environment commit failed for %s: %v", ext.DisplayName, err)
				continue
			}

			upgradesPerformed++
			log.Printf("[ExtensionSync] Hotfix successfully propagated for %s", ext.DisplayName)
		}
	}

	if upgradesPerformed > 0 {
		log.Printf("[ExtensionSync] Re-integrated system configuration with %d hotfixes injected successfully", upgradesPerformed)
	} else {
		log.Println("[ExtensionSync] Infrastructure baseline validated; all modules report absolute parity with master.")
	}
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// bootstrapExtensionConfigs synchronizes secure external environmental config declarations into extension-specific filesystem store.
func bootstrapExtensionConfigs(dataDir string) {
	// 1. Apple Music - Dynamic Security Credentials
	appleKey := strings.TrimSpace(os.Getenv("FLACAPI_APPLE_PROXY_KEY"))
	if appleKey != "" {
		updateExtensionSetting(dataDir, "apple-music", "proxyApiKey", appleKey)
	}

	// 2. Tidal Web - Personalized Endpoint Mirroring
	tidalMirror := strings.TrimSpace(os.Getenv("FLACAPI_TIDAL_MIRROR_URL"))
	tidalToken := strings.TrimSpace(os.Getenv("FLACAPI_TIDAL_TOKEN"))
	if tidalMirror != "" {
		updateExtensionSetting(dataDir, "tidal-web", "downloadApiUrl", tidalMirror)
	}
	if tidalToken != "" {
		updateExtensionSetting(dataDir, "tidal-web", "publicToken", tidalToken)
	}
}

// updateExtensionSetting handles secure transactional merge of individual dynamic key mappings into core static setting manifest.
func updateExtensionSetting(dataDir, extID, key string, val interface{}) {
	targetPath := filepath.Join(dataDir, extID, "settings.json")
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)

	// Read existing configuration frame to preserve tangential local overrides
	configMap := map[string]interface{}{"_enabled": true}
	if rawBytes, err := os.ReadFile(targetPath); err == nil {
		_ = json.Unmarshal(rawBytes, &configMap)
	}

	// Overwrite discrete target binding
	configMap[key] = val

	// Safely persist final resolved manifest back to state store
	finalOutput, _ := json.MarshalIndent(configMap, "", "  ")
	if err := os.WriteFile(targetPath, finalOutput, 0644); err == nil {
		log.Printf("Bootstrapped dynamic setting [%s] into runtime config for '%s'.", key, extID)
	}
}
