package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gb "github.com/zarz/spotiflac_android/go_backend"
)

// DataDir is the validated root folder for all writes and reads.
// It can be customized at server startup.
var DataDir = "./data"

// SafePath checks if the given path is safe and returns the cleaned absolute or relative-to-data path.
// It prevents path traversal vulnerability.
func SafePath(unsafePath string) (string, error) {
	if unsafePath == "" {
		return "", fmt.Errorf("empty path")
	}
	cleaned := filepath.Clean(unsafePath)

	// Resolve relative paths against DataDir
	var targetPath string
	if filepath.IsAbs(cleaned) {
		targetPath = cleaned
	} else {
		targetPath = filepath.Join(DataDir, cleaned)
	}

	// Double check that the cleaned path remains within DataDir boundary
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	absDataDir, err := filepath.Abs(DataDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute data dir: %w", err)
	}

	rel, err := filepath.Rel(absDataDir, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("access denied: path '%s' is outside of allowed directory '%s'", unsafePath, DataDir)
	}

	return absTarget, nil
}

// HandleDownloadByStrategy maps to DownloadByStrategy in backend.
func HandleDownloadByStrategy(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Read Error", "message":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	var req struct {
		RequestJSON string `json:"requestJSON"`
	}
	var reqStr string
	if unmarshalErr := json.Unmarshal(bodyBytes, &req); unmarshalErr == nil && req.RequestJSON != "" {
		reqStr = req.RequestJSON
	} else {
		reqStr = string(bodyBytes)
	}

	// Resolve relative output_dir to absolute path to guarantee consistent folder output across all JS extensions
	var reqMap map[string]interface{}
	if mapUnmarshalErr := json.Unmarshal([]byte(reqStr), &reqMap); mapUnmarshalErr == nil {
		if outDir, ok := reqMap["output_dir"].(string); ok && outDir != "" {
			if absOutDir, absPathErr := filepath.Abs(outDir); absPathErr == nil {
				reqMap["output_dir"] = absOutDir
				if updatedBytes, marshalErr := json.Marshal(reqMap); marshalErr == nil {
					reqStr = string(updatedBytes)
				}
			}
		}
	}

	result, err := gb.DownloadByStrategy(reqStr)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Auto-convert to .flac if the backend returned an .m4a ALAC container
	var respMap map[string]interface{}
	if json.Unmarshal([]byte(result), &respMap) == nil {
		success, _ := respMap["success"].(bool)
		filePath, _ := respMap["file_path"].(string)
		coverURL, _ := respMap["cover_url"].(string)
		if success && filePath != "" && strings.HasSuffix(strings.ToLower(filePath), ".m4a") {
			targetFlacPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".flac"
			cmd := exec.Command("ffmpeg", "-i", filePath, "-c:a", "flac", "-y", targetFlacPath)
			if transcodeErr := cmd.Run(); transcodeErr == nil {
				_ = os.Remove(filePath)

				// Programmatically download and natively embed the cover art as an attached picture in the FLAC metadata
				if coverURL != "" {
					if httpResp, httpErr := http.Get(coverURL); httpErr == nil && httpResp.StatusCode == http.StatusOK {
						tempCoverPath := targetFlacPath + ".cover.jpg"
						if coverFile, createErr := os.Create(tempCoverPath); createErr == nil {
							_, _ = io.Copy(coverFile, httpResp.Body)
							_ = coverFile.Close()
							_ = httpResp.Body.Close()

							tempFlacPath := targetFlacPath + ".temp.flac"
							embedCmd := exec.Command("ffmpeg", "-i", targetFlacPath, "-i", tempCoverPath, "-map", "0:a", "-map", "1:v", "-c:a", "copy", "-c:v", "copy", "-disposition:v", "attached_pic", "-y", tempFlacPath)
							if embedErr := embedCmd.Run(); embedErr == nil {
								_ = os.Remove(targetFlacPath)
								_ = os.Rename(tempFlacPath, targetFlacPath)
							}
							_ = os.Remove(tempCoverPath)
						}
					}
				}

				respMap["file_path"] = targetFlacPath
				respMap["requires_container_conversion"] = false
				respMap["actual_extension"] = ".flac"
				respMap["actual_container"] = "FLAC"
				if updatedResultBytes, marshalErr := json.Marshal(respMap); marshalErr == nil {
					result = string(updatedResultBytes)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleGetDownloadProgress retrieves single progress.
func HandleGetDownloadProgress(w http.ResponseWriter, r *http.Request) {
	result := gb.GetDownloadProgress()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleGetAllDownloadProgress retrieves all active progress.
func HandleGetAllDownloadProgress(w http.ResponseWriter, r *http.Request) {
	result := gb.GetAllDownloadProgress()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleGetAllDownloadProgressDelta retrieves delta updates since a sequence number.
func HandleGetAllDownloadProgressDelta(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	var sinceSeq int64
	if sinceStr != "" {
		_, _ = fmt.Sscanf(sinceStr, "%d", &sinceSeq)
	}
	result := gb.GetAllDownloadProgressDelta(sinceSeq)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleInitItemProgress initializes progress tracking for a given item.
func HandleInitItemProgress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemID string `json:"itemId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ItemID == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"itemId is required"}`, http.StatusBadRequest)
		return
	}
	gb.InitItemProgress(req.ItemID)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

// HandleFinishItemProgress marks item progress as completed.
func HandleFinishItemProgress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemID string `json:"itemId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ItemID == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"itemId is required"}`, http.StatusBadRequest)
		return
	}
	gb.FinishItemProgress(req.ItemID)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

// HandleClearItemProgress removes progress details for a finished item.
func HandleClearItemProgress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemID string `json:"itemId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ItemID == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"itemId is required"}`, http.StatusBadRequest)
		return
	}
	gb.ClearItemProgress(req.ItemID)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

// HandleCancelDownload cancels an ongoing download.
func HandleCancelDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemID string `json:"itemId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ItemID == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"itemId is required"}`, http.StatusBadRequest)
		return
	}
	gb.CancelDownload(req.ItemID)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

// HandleReadMetadata reads and returns metadata of a file.
func HandleReadMetadata(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FilePath string `json:"filePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FilePath == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"filePath is required"}`, http.StatusBadRequest)
		return
	}
	safePath, err := SafePath(req.FilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	result, err := gb.ReadFileMetadata(safePath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleEditMetadata updates metadata of a file.
func HandleEditMetadata(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FilePath     string          `json:"filePath"`
		MetadataJSON json.RawMessage `json:"metadataJSON"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FilePath == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"filePath is required"}`, http.StatusBadRequest)
		return
	}
	safePath, err := SafePath(req.FilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	result, err := gb.EditFileMetadata(safePath, string(req.MetadataJSON))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleDownloadCover downloads cover art into a safe local file.
func HandleDownloadCover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CoverURL   string `json:"coverUrl"`
		OutputPath string `json:"outputPath"`
		MaxQuality bool   `json:"maxQuality"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CoverURL == "" || req.OutputPath == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"coverUrl and outputPath are required"}`, http.StatusBadRequest)
		return
	}
	safePath, err := SafePath(req.OutputPath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	err = gb.DownloadCoverToFile(req.CoverURL, safePath, req.MaxQuality)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

// HandleGetLyrics retrieves lyrics for a track.
func HandleGetLyrics(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpotifyID  string `json:"spotifyId"`
		TrackName  string `json:"trackName"`
		ArtistName string `json:"artistName"`
		FilePath   string `json:"filePath"`
		DurationMs int64  `json:"durationMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Invalid JSON", "message":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	var safePath string
	var err error
	if req.FilePath != "" {
		safePath, err = SafePath(req.FilePath)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
			return
		}
	}
	result, err := gb.GetLyricsLRCWithSource(req.SpotifyID, req.TrackName, req.ArtistName, safePath, req.DurationMs)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleEmbedLyrics embeds lyrics directly to a music file.
func HandleEmbedLyrics(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FilePath string `json:"filePath"`
		Lyrics   string `json:"lyrics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FilePath == "" || req.Lyrics == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"filePath and lyrics are required"}`, http.StatusBadRequest)
		return
	}
	safePath, err := SafePath(req.FilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	result, err := gb.EmbedLyricsToFile(safePath, req.Lyrics)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleSetDownloadDir configures the default download directory safely.
func HandleSetDownloadDir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"path is required"}`, http.StatusBadRequest)
		return
	}
	safePath, err := SafePath(req.Path)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	err = gb.SetDownloadDirectory(safePath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	gb.AllowDownloadDir(safePath)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

// HandleCustomSearch triggers CustomSearchWithExtensionJSON in the backend.
func HandleCustomSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Service string          `json:"service"`
		Query   string          `json:"query"`
		Options json.RawMessage `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" || req.Query == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"service and query are required"}`, http.StatusBadRequest)
		return
	}
	result, err := gb.CustomSearchWithExtensionJSON(req.Service, req.Query, string(req.Options))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Search Failed", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleCheckAvailability checks if a track is available based on Spotify ID and/or ISRC.
func HandleCheckAvailability(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpotifyID string `json:"spotifyId"`
		ISRC      string `json:"isrc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Invalid JSON", "message":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	result, err := gb.CheckAvailability(req.SpotifyID, req.ISRC)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleCheckAvailabilityByPlatform checks if a track/album/playlist is available on a specific platform.
func HandleCheckAvailabilityByPlatform(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Platform   string `json:"platform"`
		EntityType string `json:"entityType"`
		EntityID   string `json:"entityId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Platform == "" || req.EntityType == "" || req.EntityID == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"platform, entityType and entityId are required"}`, http.StatusBadRequest)
		return
	}
	result, err := gb.CheckAvailabilityByPlatformID(req.Platform, req.EntityType, req.EntityID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleResolveID converts a Spotify Track/Album ID to a Deezer equivalent ID.
func HandleResolveID(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceType string `json:"resourceType"`
		SpotifyID    string `json:"spotifyId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ResourceType == "" || req.SpotifyID == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"resourceType and spotifyId are required"}`, http.StatusBadRequest)
		return
	}
	result, err := gb.ConvertSpotifyToDeezer(req.ResourceType, req.SpotifyID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleGetProviderMetadata fetches resource metadata from any supported provider.
func HandleGetProviderMetadata(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProviderID   string `json:"providerId"`
		ResourceType string `json:"resourceType"`
		ResourceID   string `json:"resourceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProviderID == "" || req.ResourceType == "" || req.ResourceID == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"providerId, resourceType and resourceId are required"}`, http.StatusBadRequest)
		return
	}
	result, err := gb.GetProviderMetadataJSON(req.ProviderID, req.ResourceType, req.ResourceID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleCheckDuplicate checks if a track is already downloaded in the output directory.
func HandleCheckDuplicate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OutputDir string `json:"outputDir"`
		ISRC      string `json:"isrc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OutputDir == "" || req.ISRC == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"outputDir and isrc are required"}`, http.StatusBadRequest)
		return
	}
	safeOutputDir, err := SafePath(req.OutputDir)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	result, err := gb.CheckDuplicate(safeOutputDir, req.ISRC)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleCheckDuplicatesBatch checks duplication for a batch of tracks.
func HandleCheckDuplicatesBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OutputDir  string          `json:"outputDir"`
		TracksJSON json.RawMessage `json:"tracksJSON"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OutputDir == "" || len(req.TracksJSON) == 0 {
		http.Error(w, `{"error":"Invalid JSON", "message":"outputDir and tracksJSON are required"}`, http.StatusBadRequest)
		return
	}
	safeOutputDir, err := SafePath(req.OutputDir)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	result, err := gb.CheckDuplicatesBatch(safeOutputDir, string(req.TracksJSON))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}

// HandleExtractCover extracts embedded cover art from an audio file.
func HandleExtractCover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AudioPath  string `json:"audioPath"`
		OutputPath string `json:"outputPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AudioPath == "" || req.OutputPath == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"audioPath and outputPath are required"}`, http.StatusBadRequest)
		return
	}
	safeAudio, err := SafePath(req.AudioPath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	safeOutput, err := SafePath(req.OutputPath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	err = gb.ExtractCoverToFile(safeAudio, safeOutput)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

// HandleParseCueSheet parses a local .cue sheet file.
func HandleParseCueSheet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CuePath  string `json:"cuePath"`
		AudioDir string `json:"audioDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CuePath == "" || req.AudioDir == "" {
		http.Error(w, `{"error":"Invalid JSON", "message":"cuePath and audioDir are required"}`, http.StatusBadRequest)
		return
	}
	safeCue, err := SafePath(req.CuePath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	safeAudioDir, err := SafePath(req.AudioDir)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	result, err := gb.ParseCueSheet(safeCue, safeAudioDir)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
}
