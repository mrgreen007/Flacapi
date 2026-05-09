package main

import (
	"context"
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

	"sabuj.in/flacapi/internal/api"
	go_backend "github.com/zarz/spotiflac_android/go_backend"
)

func main() {
	// Retrieve paths from environment or use defaults
	dataDir := os.Getenv("FLACAPI_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	extensionsDir := os.Getenv("FLACAPI_EXTENSIONS_DIR")
	if extensionsDir == "" {
		extensionsDir = "./extensions"
	}

	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		log.Fatalf("Invalid data directory path %s: %v", dataDir, err)
	}
	absExtDir, err := filepath.Abs(extensionsDir)
	if err != nil {
		log.Fatalf("Invalid extensions directory path %s: %v", extensionsDir, err)
	}

	// Ensure directories exist
	if err := os.MkdirAll(absDataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory %s: %v", absDataDir, err)
	}
	if err := os.MkdirAll(absExtDir, 0755); err != nil {
		log.Fatalf("Failed to create extensions directory %s: %v", absExtDir, err)
	}

	// Determine active source extensions directory
	srcExtDir := absExtDir
	if _, err := os.Stat(filepath.Join(absExtDir, "extensions")); err == nil {
		srcExtDir = filepath.Join(absExtDir, "extensions")
	}

	// Point run-time extensions to git-ignored data subdirectory to prevent dirtying submodules
	runExtDir := filepath.Join(absDataDir, "extensions_run")
	_ = os.RemoveAll(runExtDir)
	if err := os.MkdirAll(runExtDir, 0755); err != nil {
		log.Fatalf("Failed to create run-time extensions directory %s: %v", runExtDir, err)
	}

	// Copy .spotiflac-ext packages to the run-time directory
	if err := copyExtensions(srcExtDir, runExtDir); err != nil {
		log.Printf("Warning: Failed to copy extension packages from %s to %s: %v", srcExtDir, runExtDir, err)
	}

	// Configure handler base directory
	api.DataDir = absDataDir

	// Initialize backend download folder
	if err := go_backend.SetDownloadDirectory(absDataDir); err != nil {
		log.Printf("Warning: Failed to set default download directory: %v", err)
	}
	go_backend.AllowDownloadDir(absDataDir)

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

			// Configure default fallback priorities to support automatic multi-provider lossless downloads (e.g. from Spotify metadata bootstrap)
			_ = go_backend.SetProviderPriorityJSON(`["tidal-web", "apple-music", "qobuz-web", "deezer", "ytmusic-spotiflac", "amazon", "soundcloud", "pandora"]`)
			_ = go_backend.SetExtensionFallbackProviderIDsJSON(`["tidal-web", "apple-music", "qobuz-web", "deezer", "ytmusic-spotiflac", "amazon", "soundcloud", "pandora"]`)
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
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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