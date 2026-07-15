package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	gb "github.com/zarz/spotiflac_android/go_backend"
)

const (
	qualityLossless   = "LOSSLESS"
	statusPreparing   = "preparing"
	statusDownloading = "downloading"
	statusFinalizing  = "finalizing"
	statusCompleted   = "completed"
	statusFailed      = "failed"
)

// DownloadState tracks asynchronous background download tasks
type DownloadState struct {
	ItemID       string    `json:"itemId"`
	Status       string    `json:"status"` // preparing, downloading, finalizing, completed, failed
	Error        string    `json:"error,omitempty"`
	FilePath     string    `json:"-"`
	CreatedAt    time.Time `json:"-"`
	LastAccessed time.Time `json:"-"`
}

// DownloadStates is a thread-safe map tracking all active and completed downloads
var DownloadStates sync.Map // map[string]*DownloadState

type persistentDownloadState struct {
	ItemID       string    `json:"itemId"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	FilePath     string    `json:"filePath"`
	CreatedAt    time.Time `json:"createdAt"`
	LastAccessed time.Time `json:"lastAccessed"`
}

func saveDownloadStates() {
	states := make(map[string]*persistentDownloadState)
	DownloadStates.Range(func(key, value interface{}) bool {
		s := value.(*DownloadState)
		states[key.(string)] = &persistentDownloadState{
			ItemID:       s.ItemID,
			Status:       s.Status,
			Error:        s.Error,
			FilePath:     s.FilePath,
			CreatedAt:    s.CreatedAt,
			LastAccessed: s.LastAccessed,
		}
		return true
	})

	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		log.Printf("[Storage] Failed to marshal download states: %v", err)
		return
	}

	filePath := filepath.Join(DataDir, "download_states.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		log.Printf("[Storage] Failed to write download states file: %v", err)
	}
}

func LoadDownloadStates() {
	filePath := filepath.Join(DataDir, "download_states.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("[Storage] Failed to read download states: %v", err)
		return
	}

	var states map[string]*persistentDownloadState
	if err := json.Unmarshal(data, &states); err != nil {
		log.Printf("[Storage] Failed to unmarshal download states: %v", err)
		return
	}

	for k, v := range states {
		// Clean up any states that were interrupted during download or finalization
		if v.Status == statusPreparing || v.Status == statusDownloading || v.Status == statusFinalizing {
			v.Status = statusFailed
			v.Error = "Download interrupted by server restart"

			// Clean up staging directory associated with the interrupted task
			stagingDir := filepath.Join(os.TempDir(), "spotiflac_staging", v.ItemID)
			_ = os.RemoveAll(stagingDir)
		}

		DownloadStates.Store(k, &DownloadState{
			ItemID:       v.ItemID,
			Status:       v.Status,
			Error:        v.Error,
			FilePath:     v.FilePath,
			CreatedAt:    v.CreatedAt,
			LastAccessed: v.LastAccessed,
		})
	}

	// Persist the updated fail statuses
	saveDownloadStates()
}

// DataDir is the primary root folder for app data and default relative path resolving.
var DataDir = "./data"

// DefaultDownloadDir is the active fallback directory for audio downloads if not specified by payload.
var DefaultDownloadDir = "./downloads"

// ConversionStrategy dictates handling of containers returned by the backend.
// Accepted values: "ORIGINAL" (default, preserves native extensions like .m4a), "FORCE_FLAC".
var ConversionStrategy = "ORIGINAL"

// AdditionalAllowedDirs stores extra paths (like separate mounted downloads volume) to permit access.
var AdditionalAllowedDirs []string

func isWithinDir(baseDir, targetPath string) bool {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}
	// Validates that resolved relative path does not move UP using ".." and is not the ".." itself
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// SafePath checks if the given path is safe and returns the cleaned absolute or relative-to-data path.
// It prevents path traversal and enforces boundaries against DataDir and AdditionalAllowedDirs.
func SafePath(unsafePath string) (string, error) {
	if unsafePath == "" {
		return "", fmt.Errorf("empty path")
	}
	cleaned := filepath.Clean(unsafePath)

	// Resolve relative paths against primary DataDir
	var targetPath string
	if filepath.IsAbs(cleaned) {
		targetPath = cleaned
	} else {
		targetPath = filepath.Join(DataDir, cleaned)
	}

	// Resolve to absolute representation
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Verify bound verification against default DataDir
	if isWithinDir(DataDir, absTarget) {
		return absTarget, nil
	}

	// Verify bound verification against other permitted paths (like a separate downloads volume)
	for _, dir := range AdditionalAllowedDirs {
		if dir != "" && isWithinDir(dir, absTarget) {
			return absTarget, nil
		}
	}

	return "", fmt.Errorf("access denied: path is outside of allowed directory boundaries")
}

func updateStateStatus(itemID, status, errStr string) {
	if val, ok := DownloadStates.Load(itemID); ok {
		state := val.(*DownloadState)
		state.Status = status
		state.Error = errStr
		state.LastAccessed = time.Now()
		saveDownloadStates()
	}
}



func updateStateFailed(itemID, errStr string) {
	if val, ok := DownloadStates.Load(itemID); ok {
		state := val.(*DownloadState)
		state.Status = statusFailed
		state.Error = errStr
		state.LastAccessed = time.Now()
		saveDownloadStates()
	}
}

func updateStateCompleted(itemID, filePath string) {
	if val, ok := DownloadStates.Load(itemID); ok {
		state := val.(*DownloadState)
		state.Status = statusCompleted
		state.FilePath = filePath
		state.LastAccessed = time.Now()
		saveDownloadStates()
	}
}

func runAsyncDownload(itemID string, reqMap map[string]interface{}, finalTargetDir string, reqConvStrat string) {
	// 1. Setup unique staging directory
	stagingDir := filepath.Join(os.TempDir(), "spotiflac_staging", itemID)
	_ = os.RemoveAll(stagingDir) // Clean up any leftover
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		updateStateFailed(itemID, fmt.Sprintf("Failed to create staging directory: %v", err))
		return
	}
	defer os.RemoveAll(stagingDir)

	// Override the output_dir to our unique staging directory
	reqMap["output_dir"] = stagingDir

	// Convert reqMap back to string for backend dispatch
	reqBytes, err := json.Marshal(reqMap)
	if err != nil {
		updateStateFailed(itemID, fmt.Sprintf("Failed to encode request: %v", err))
		return
	}
	reqStr := string(reqBytes)

	// Read parameters for quality validation & fallback
	quality, _ := reqMap["quality"].(string)
	useFallback, _ := reqMap["use_fallback"].(bool)
	requestedService, _ := reqMap["service"].(string)

	var result string
	var downloadErr error

	updateStateStatus(itemID, "downloading", "")

	// Fallback Loop for Lossless
	if isLosslessRequest(quality) && useFallback {
		providers, pErr := getFilteredLosslessPriority(requestedService)
		if pErr == nil && len(providers) > 0 {
			// Set use_fallback to false so go_backend doesn't run its own internal fallback
			reqMap["use_fallback"] = false
			successFound := false
			var lastResult string
			var lastErr error

			for _, provider := range providers {
				reqMap["service"] = provider
				updatedBytes, marshalErr := json.Marshal(reqMap)
				if marshalErr != nil {
					continue
				}

				loopResult, loopErr := gb.DownloadByStrategy(string(updatedBytes))
				lastResult = loopResult
				lastErr = loopErr

				if loopErr == nil {
					var rMap map[string]interface{}
					if json.Unmarshal([]byte(loopResult), &rMap) == nil {
						filePath := ""
						if fp, ok := rMap["FilePath"].(string); ok && fp != "" {
							filePath = fp
						} else if fp, ok := rMap["file_path"].(string); ok && fp != "" {
							filePath = fp
						}

						if filePath != "" {
							// Check codec
							if isLosslessCodec(filePath) {
								result = loopResult
								downloadErr = nil
								successFound = true
								break
							} else {
								_ = os.Remove(filePath)
								log.Printf("[DownloadAsync] Provider %s returned lossy container for lossless request. Discarding and seeking fallback...", provider)
							}
						}
					}
				}
			}

			if !successFound {
				result = lastResult
				downloadErr = lastErr
				if result == "" && downloadErr == nil {
					downloadErr = fmt.Errorf("all lossless providers failed to resolve this track")
				}
			}
		} else {
			result, downloadErr = gb.DownloadByStrategy(reqStr)
		}
	} else {
		result, downloadErr = gb.DownloadByStrategy(reqStr)
	}

	if downloadErr != nil {
		updateStateFailed(itemID, fmt.Sprintf("Download failed: %v", downloadErr))
		return
	}

	// Now parse result JSON
	var respMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &respMap); err != nil {
		updateStateFailed(itemID, fmt.Sprintf("Invalid response from backend: %v", err))
		return
	}

	// Check success field (backend might return PascalCase or lowercase, let's support both)
	success := false
	if s, ok := respMap["success"].(bool); ok {
		success = s
	} else if s, ok := respMap["Success"].(bool); ok {
		success = s
	}

	filePath := ""
	if fp, ok := respMap["file_path"].(string); ok && fp != "" {
		filePath = fp
	} else if fp, ok := respMap["FilePath"].(string); ok && fp != "" {
		filePath = fp
	}

	if !success || filePath == "" {
		errMsg := "Unknown backend error"
		if msg, ok := respMap["error"].(string); ok && msg != "" {
			errMsg = msg
		}
		updateStateFailed(itemID, errMsg)
		return
	}

	// Transition state to finalizing
	updateStateStatus(itemID, "finalizing", "")
	gb.SetItemFinalizing(itemID)

	// Final Lossless Verification (if requested lossless)
	if isLosslessRequest(quality) {
		if !isLosslessCodec(filePath) {
			_ = os.Remove(filePath)
			updateStateFailed(itemID, "quality_rejected: Provided stream failed final lossless assertion test")
			return
		}
	}

	coverURL := getStringAny(respMap, "cover_url", "CoverURL")
	finalFilePath := filePath

	// 1. CONDITIONAL TRANSCODE: Convert ALAC (M4A) to FLAC if requested FORCE_FLAC and it's genuine lossless
	if strings.HasSuffix(strings.ToLower(finalFilePath), ".m4a") && strings.ToUpper(reqConvStrat) != "ORIGINAL" {
		// Double check it's lossless ALAC before transcoding (never force flac on lossy audio)
		if isLosslessCodec(finalFilePath) {
			targetFlacPath := strings.TrimSuffix(finalFilePath, filepath.Ext(finalFilePath)) + ".flac"
			var stderr bytes.Buffer
			cmd := exec.Command("ffmpeg", "-i", finalFilePath, "-c:a", "flac", "-y", targetFlacPath)
			cmd.Stderr = &stderr
			if err := cmd.Run(); err == nil {
				_ = os.Remove(finalFilePath)
				finalFilePath = targetFlacPath
				respMap["file_path"] = finalFilePath
				respMap["actual_extension"] = ".flac"
				respMap["actual_container"] = "FLAC"
				respMap["requires_container_conversion"] = false
			} else {
				log.Printf("[DownloadAsync] ALAC to FLAC transcode failed: %v, stderr: %s", err, stderr.String())
				// Proceed with the original ALAC M4A rather than failing completely
			}
		}
	}

	// 2. UNIFIED METADATA & COVER INJECTION
	fileExt := filepath.Ext(finalFilePath)
	tempOutPath := finalFilePath + ".temp_final" + fileExt
	baseArgs := []string{"-i", finalFilePath}
	hasCover := false
	tempCoverPath := finalFilePath + ".cover.jpg"

	if coverURL != "" {
		// Cover Art Download with dedicated Client timeout
		coverClient := &http.Client{Timeout: 10 * time.Second}
		if httpResp, httpErr := coverClient.Get(coverURL); httpErr == nil && httpResp.StatusCode == http.StatusOK {
			if coverFile, createErr := os.Create(tempCoverPath); createErr == nil {
				_, _ = io.Copy(coverFile, httpResp.Body)
				_ = coverFile.Close()
				_ = httpResp.Body.Close()
				baseArgs = append(baseArgs, "-i", tempCoverPath)
				hasCover = true
			}
		} else {
			// Log warning but proceed with tagging
			log.Printf("[DownloadAsync] Cover art download failed: %v. Proceeding without cover.", httpErr)
		}
	}

	// Construct stream mapping depending on format
	baseArgs = append(baseArgs, "-map", "0:a")
	if hasCover {
		// Note: Newer FFmpeg versions support disposition for MP4 cover as well.
		baseArgs = append(baseArgs, "-map", "1:v", "-disposition:v", "attached_pic")
	}

	// Transcode cover art to mjpeg (JPEG) for container compatibility (WebP, PNG, etc.)
	if hasCover {
		baseArgs = append(baseArgs, "-c:a", "copy", "-c:v", "mjpeg")
	} else {
		baseArgs = append(baseArgs, "-c:a", "copy")
	}

	// Tag merging
	addMeta := func(ffmpegKey string, keys ...string) {
		val := getStringAny(respMap, keys...)
		if strings.TrimSpace(val) != "" {
			baseArgs = append(baseArgs, "-metadata", fmt.Sprintf("%s=%s", ffmpegKey, val))
		}
	}
	addMetaNum := func(ffmpegKey string, keys ...string) {
		val := getFloatAny(respMap, keys...)
		if val > 0 {
			baseArgs = append(baseArgs, "-metadata", fmt.Sprintf("%s=%d", ffmpegKey, int(val)))
		}
	}

	addMeta("title", "title", "Title")
	addMeta("artist", "artist", "Artist")
	addMeta("album", "album", "Album")
	addMeta("album_artist", "album_artist", "AlbumArtist")
	addMeta("date", "release_date", "ReleaseDate")
	addMeta("genre", "genre", "Genre")
	addMeta("copyright", "copyright", "Copyright")
	addMeta("composer", "composer", "Composer")
	addMeta("comment", "isrc", "ISRC")
	addMetaNum("track", "track_number", "TrackNumber")
	addMetaNum("disc", "disc_number", "DiscNumber")

	if strings.ToLower(fileExt) == ".m4a" {
		baseArgs = append(baseArgs, "-f", "mp4")
	}
	baseArgs = append(baseArgs, "-y", tempOutPath)

	var ffmpegStderr bytes.Buffer
	embedCmd := exec.Command("ffmpeg", baseArgs...)
	embedCmd.Stderr = &ffmpegStderr

	if embedCmd.Run() == nil {
		_ = os.Remove(finalFilePath)
		_ = os.Rename(tempOutPath, finalFilePath)
	} else {
		log.Printf("[DownloadAsync] FFmpeg tagging failed: %s", ffmpegStderr.String())
		_ = os.Remove(tempOutPath)
	}

	if hasCover {
		_ = os.Remove(tempCoverPath)
	}

	// 3. MOVE TO FINAL DIRECTORY (completely managed internally by server)
	_ = os.MkdirAll(finalTargetDir, 0755)
	baseName := filepath.Base(finalFilePath)
	absoluteDest := filepath.Join(finalTargetDir, baseName)

	if err := os.Rename(finalFilePath, absoluteDest); err == nil {
		finalFilePath = absoluteDest
	} else {
		if copyErr := safeStreamCopy(finalFilePath, absoluteDest); copyErr == nil {
			_ = os.Remove(finalFilePath)
			finalFilePath = absoluteDest
		} else {
			updateStateFailed(itemID, fmt.Sprintf("Failed to relocate finalized file: %v", copyErr))
			return
		}
	}

	// Completed successfully!
	updateStateCompleted(itemID, finalFilePath)
	gb.CompleteItemProgress(itemID)
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

	var reqMap map[string]interface{}
	if mapUnmarshalErr := json.Unmarshal([]byte(reqStr), &reqMap); mapUnmarshalErr != nil {
		reqMap = make(map[string]interface{})
	}

	// Premium Enhancement: Transparently resolve missing metadata names from ISRC before downloader proceeds
	enrichRequestMetadataFromISRC(reqMap)

	// Smart Defaults: Ensure maximum features & reliable provider chaining is enabled by default
	booleanDefaults := []string{"embed_metadata", "embed_max_quality_cover", "embed_lyrics", "use_fallback"}
	for _, key := range booleanDefaults {
		if _, exists := reqMap[key]; !exists {
			reqMap[key] = true
		}
	}

	// Item ID allocation
	itemID, _ := reqMap["item_id"].(string)
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		itemID = fmt.Sprintf("dl-%d", time.Now().UnixMilli())
		reqMap["item_id"] = itemID
	}

	// Quality default
	if q, exists := reqMap["quality"]; !exists || strings.TrimSpace(fmt.Sprintf("%v", q)) == "" {
		reqMap["quality"] = qualityLossless
	}

	// Safety Upgrade: Scrub all user-supplied inputs to strip unsafe characters before backend dispatch
	fieldsToScrub := []string{"track_name", "artist_name", "album_name"}
	for _, f := range fieldsToScrub {
		if val, ok := reqMap[f].(string); ok && strings.TrimSpace(val) != "" {
			reqMap[f] = sanitizeMetadataString(val)
		}
	}

	// Parse custom strategy parameter
	reqConvStrat := ConversionStrategy
	if strat, ok := reqMap["conversion_strategy"].(string); ok && strat != "" {
		reqConvStrat = strings.TrimSpace(strat)
	}

	// Discontinue client specified output directory - managed entirely internally
	var finalTargetDir string
	if absOutDir, absPathErr := filepath.Abs(DefaultDownloadDir); absPathErr == nil {
		finalTargetDir = absOutDir
	} else {
		finalTargetDir = DefaultDownloadDir
	}

	// Store initial state
	DownloadStates.Store(itemID, &DownloadState{
		ItemID:       itemID,
		Status:       "preparing",
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	})
	saveDownloadStates()

	// Register progress tracking in the backend
	gb.InitItemProgress(itemID)

	// Launch background download task
	go runAsyncDownload(itemID, reqMap, finalTargetDir, reqConvStrat)

	// Respond immediately with Accepted status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"itemId":"%s","status":"preparing"}`, itemID)))
}

// itemProgressRaw is a helper structure to parse go_backend progress items
type itemProgressRaw struct {
	ItemID        string  `json:"item_id"`
	Progress      float64 `json:"progress"`
	IsDownloading bool    `json:"is_downloading"`
	Status        string  `json:"status"`
}

// multiProgressRaw is a helper structure to parse go_backend multi-progress responses
type multiProgressRaw struct {
	Items map[string]*itemProgressRaw `json:"items"`
}

// multiProgressDeltaRaw is a helper structure to parse go_backend multi-progress delta responses
type multiProgressDeltaRaw struct {
	Seq     int64                       `json:"seq"`
	Reset   bool                        `json:"reset,omitempty"`
	Items   map[string]*itemProgressRaw `json:"items,omitempty"`
	Removed []string                    `json:"removed,omitempty"`
}

func roundToOneDecimal(v float64) float64 {
	return math.Round(v*10) / 10
}

// HandleGetDownloadProgress retrieves single progress.
func HandleGetDownloadProgress(w http.ResponseWriter, r *http.Request) {
	itemID := r.URL.Query().Get("itemId")
	if itemID == "" {
		itemID = r.URL.Query().Get("item_id")
	}

	if itemID == "" {
		http.Error(w, `{"error":"Missing Parameter", "message":"itemId is required"}`, http.StatusBadRequest)
		return
	}

	// Fetch backend's multi progress
	allProgressJSON := gb.GetAllDownloadProgress()

	var parsed multiProgressRaw
	_ = json.Unmarshal([]byte(allProgressJSON), &parsed)

	// Local State Lookup
	val, existsLocal := DownloadStates.Load(itemID)
	if !existsLocal {
		// If backend doesn't have it either, return 404
		if parsed.Items == nil || parsed.Items[itemID] == nil {
			http.Error(w, `{"error":"Not Found", "message":"item not found"}`, http.StatusNotFound)
			return
		}
	}

	// Prepare result map
	resMap := map[string]interface{}{
		"item_id":        itemID,
		"status":         statusPreparing,
		"progress":       0.0,
		"is_downloading": false,
	}

	// Populate local state metadata
	if existsLocal {
		state := val.(*DownloadState)
		resMap["status"] = state.Status
		if state.Error != "" {
			resMap["error"] = state.Error
		}
		if state.Status == statusCompleted {
			resMap["progress"] = 100.0
		} else if state.Status == statusFinalizing {
			resMap["progress"] = 100.0
		}
	}

	// Merge backend tracking metrics if available
	if parsed.Items != nil {
		if backendItem, found := parsed.Items[itemID]; found {
			resMap["is_downloading"] = backendItem.IsDownloading
			// If our local state says "finalizing" or "completed" or "failed", keep that ground truth.
			// Otherwise use the backend status ("preparing", "downloading", "completed", "finalizing").
			if existsLocal {
				state := val.(*DownloadState)
				if state.Status == statusPreparing || state.Status == statusDownloading {
					resMap["status"] = backendItem.Status
					resMap["progress"] = roundToOneDecimal(backendItem.Progress * 100)
				}
			} else {
				resMap["status"] = backendItem.Status
				resMap["progress"] = roundToOneDecimal(backendItem.Progress * 100)
			}
		}
	}

	// Append any active pending auth requests to help the client auto-authenticate
	if rawJSON, err := gb.GetAllPendingAuthRequestsJSON(); err == nil {
		var authList []map[string]interface{}
		if err := json.Unmarshal([]byte(rawJSON), &authList); err == nil && len(authList) > 0 {
			resMap["pending_auth"] = authList
		}
	}

	resJSON, err := json.Marshal(resMap)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Marshal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resJSON)
}



// HandleGetAllDownloadProgressDelta retrieves delta updates since a sequence number.
func HandleGetAllDownloadProgressDelta(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	var sinceSeq int64
	if sinceStr != "" {
		_, _ = fmt.Sscanf(sinceStr, "%d", &sinceSeq)
	}
	result := gb.GetAllDownloadProgressDelta(sinceSeq)

	var parsed multiProgressDeltaRaw
	if err := json.Unmarshal([]byte(result), &parsed); err == nil && parsed.Items != nil {
		for id, item := range parsed.Items {
			if val, ok := DownloadStates.Load(id); ok {
				state := val.(*DownloadState)
				item.Status = state.Status
				if state.Status == statusCompleted || state.Status == statusFailed || state.Status == statusFinalizing {
					item.Progress = 100.0
				} else {
					item.Progress = roundToOneDecimal(item.Progress * 100)
				}
			} else {
				item.Progress = roundToOneDecimal(item.Progress * 100)
			}
		}
		if outputBytes, err := json.Marshal(parsed); err == nil {
			result = string(outputBytes)
		}
	}

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

// HandleGetLyrics retrieves lyrics for a track.
func HandleGetLyrics(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpotifyID  string `json:"spotifyId"`
		TrackName  string `json:"trackName"`
		ArtistName string `json:"artistName"`
		DurationMs int64  `json:"durationMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Invalid JSON", "message":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	result, err := gb.GetLyricsLRCWithSource(req.SpotifyID, req.TrackName, req.ArtistName, "", req.DurationMs)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Internal Error", "message":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(result))
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

// HandleDownloadFile streams the completed download file to the client and deletes it from disk.
func HandleDownloadFile(w http.ResponseWriter, r *http.Request) {
	itemID := r.URL.Query().Get("itemId")
	if itemID == "" {
		itemID = r.URL.Query().Get("item_id")
	}

	if itemID == "" {
		http.Error(w, `{"error":"Missing Parameter", "message":"itemId is required"}`, http.StatusBadRequest)
		return
	}

	// Local State Lookup
	val, exists := DownloadStates.Load(itemID)
	if !exists {
		http.Error(w, `{"error":"Not Found", "message":"item not found"}`, http.StatusNotFound)
		return
	}

	state := val.(*DownloadState)
	if state.Status != statusCompleted {
		if state.Status == statusFailed {
			http.Error(w, fmt.Sprintf(`{"error":"Download Failed", "message":"%s"}`, state.Error), http.StatusGone)
		} else {
			http.Error(w, `{"error":"Download In Progress", "message":"file is not ready yet"}`, http.StatusConflict)
		}
		return
	}

	// Resolve & Verify Safe Path
	safePath, err := SafePath(state.FilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Security Error", "message":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}

	// Verify file exists on disk
	if _, err := os.Stat(safePath); os.IsNotExist(err) {
		http.Error(w, `{"error":"Not Found", "message":"file does not exist on disk"}`, http.StatusNotFound)
		return
	}

	// Set headers
	baseName := filepath.Base(safePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, baseName))
	w.Header().Set("Content-Type", "application/octet-stream")

	// Serve file
	http.ServeFile(w, r, safePath)

	// Clean up after transfer completes
	log.Printf("[DownloadFile] Completed stream delivery for %s. Deleting from disk.", itemID)
	_ = os.Remove(safePath)
	DownloadStates.Delete(itemID)
	saveDownloadStates()
}


func getLosslessExtensionIDs() (map[string]bool, error) {
	installedJSON, err := gb.GetInstalledExtensions()
	if err != nil {
		return nil, err
	}
	type minimalExtInfo struct {
		ID           string `json:"id"`
		Capabilities struct {
			DownloadFallbackTier string `json:"downloadFallbackTier"`
		} `json:"capabilities"`
	}
	var list []minimalExtInfo
	if err := json.Unmarshal([]byte(installedJSON), &list); err != nil {
		return nil, err
	}
	losslessMap := make(map[string]bool)
	for _, item := range list {
		tier := strings.ToLower(strings.TrimSpace(item.Capabilities.DownloadFallbackTier))
		if tier == "lossless" || tier == "hi_res" {
			losslessMap[item.ID] = true
		}
	}
	return losslessMap, nil
}

func getFilteredLosslessPriority(requestedService string) ([]string, error) {
	priorityJSON, err := gb.GetProviderPriorityJSON()
	if err != nil {
		return nil, err
	}
	var priority []string
	if err = json.Unmarshal([]byte(priorityJSON), &priority); err != nil {
		return nil, err
	}

	losslessMap, err := getLosslessExtensionIDs()
	if err != nil {
		return nil, err
	}

	var filtered []string
	seen := make(map[string]bool)

	requestedService = strings.TrimSpace(requestedService)
	if requestedService != "" && losslessMap[requestedService] {
		filtered = append(filtered, requestedService)
		seen[requestedService] = true
	}

	for _, p := range priority {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if losslessMap[p] {
			filtered = append(filtered, p)
			seen[p] = true
		}
	}
	return filtered, nil
}

// enrichRequestMetadataFromISRC invokes metadata lookup to resolve missing names before download begins.
func enrichRequestMetadataFromISRC(reqMap map[string]interface{}) {
	if reqMap == nil {
		return
	}
	isrc, _ := reqMap["isrc"].(string)
	isrc = strings.TrimSpace(isrc)
	if isrc == "" {
		return
	}

	tName, _ := reqMap["track_name"].(string)
	aName, _ := reqMap["artist_name"].(string)

	// Only lookup if fundamental fields are totally missing
	if strings.TrimSpace(tName) != "" && strings.TrimSpace(aName) != "" {
		return
	}

	rawMeta, err := gb.SearchDeezerByISRC(isrc)
	if err != nil {
		return // Silent fallback
	}

	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(rawMeta), &meta); err != nil {
		return
	}

	if strings.TrimSpace(tName) == "" {
		if val, ok := meta["name"].(string); ok && strings.TrimSpace(val) != "" {
			reqMap["track_name"] = sanitizeMetadataString(val)
		}
	}
	if strings.TrimSpace(aName) == "" {
		if val, ok := meta["artists"].(string); ok && strings.TrimSpace(val) != "" {
			reqMap["artist_name"] = sanitizeMetadataString(val)
		}
	}
	albName, _ := reqMap["album_name"].(string)
	if strings.TrimSpace(albName) == "" {
		if val, ok := meta["album_name"].(string); ok && strings.TrimSpace(val) != "" {
			reqMap["album_name"] = sanitizeMetadataString(val)
		}
	}
}

var winIllegalFilenameChars = regexp.MustCompile(`[<>:"/\\|?*]`)

// sanitizeMetadataString strictly removes unsafe Windows path tokens to prevent system disk write exceptions.
func sanitizeMetadataString(s string) string {
	cleaned := winIllegalFilenameChars.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(cleaned), " ")
}

// isLosslessCodec definitively probes filesystem packets to assert true lossless delivery.
func isLosslessCodec(filePath string) bool {
	if filePath == "" {
		return false
	}

	// Probe atomic data stream using core FFprobe binaries
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", filePath)
	output, err := cmd.Output()
	if err != nil {
		return false // Safe denial of unknown container integrity
	}

	codec := strings.TrimSpace(strings.ToLower(string(output)))

	// Registered canonical lossless standard formats
	return codec == "flac" || codec == "alac" || codec == "wav" || codec == "aiff" || strings.Contains(codec, "pcm")
}

func safeStreamCopy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func getStringAny(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func getFloatAny(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k].(float64); ok {
			return v
		}
		if v, ok := m[k].(int); ok {
			return float64(v)
		}
	}
	return 0
}

func isLosslessRequest(q string) bool {
	upper := strings.ToUpper(strings.TrimSpace(q))
	return upper == qualityLossless || upper == "HI_RES" || upper == "HI_RES_LOSSLESS"
}

// AuthCallbackRequest represents the JSON request body for authentication
type AuthCallbackRequest struct {
	URL   string `json:"url,omitempty"`
	Grant string `json:"grant,omitempty"`
	Code  string `json:"code,omitempty"`
	State string `json:"state,omitempty"`
}

// HandleAuthCallback handles the OAuth / signed session grant callback and returns JSON
func HandleAuthCallback(w http.ResponseWriter, r *http.Request) {
	var reqBody AuthCallbackRequest
	if r.Header.Get("Content-Type") == "application/json" && r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
	}

	grant := r.URL.Query().Get("grant")
	if grant == "" {
		grant = r.URL.Query().Get("code")
	}
	if grant == "" {
		grant = reqBody.Grant
	}
	if grant == "" {
		grant = reqBody.Code
	}

	extID := r.URL.Query().Get("state")
	if extID == "" {
		extID = reqBody.State
	}

	inputURL := r.URL.Query().Get("url")
	if inputURL == "" {
		inputURL = reqBody.URL
	}

	if inputURL != "" {
		u := strings.TrimSpace(inputURL)
		if !strings.Contains(u, "://") {
			if strings.Contains(u, "?") {
				u = "temp://dummy" + u
			} else {
				u = "temp://dummy?" + u
			}
		}
		if parsedPaste, err := url.Parse(u); err == nil {
			q := parsedPaste.Query()
			if g := q.Get("grant"); g != "" {
				grant = g
			} else if c := q.Get("code"); c != "" {
				grant = c
			}
			if s := q.Get("state"); s != "" {
				extID = s
			}
		}
		// If grant is still empty, treat the whole pasted string as the raw token directly
		if grant == "" {
			cleaned := strings.TrimSpace(inputURL)
			if cleaned != "" {
				// Strip any prefix key if they copied the full equation (e.g. grant=gr_...)
				if strings.HasPrefix(cleaned, "grant=") {
					grant = strings.TrimPrefix(cleaned, "grant=")
				} else if strings.HasPrefix(cleaned, "code=") {
					grant = strings.TrimPrefix(cleaned, "code=")
				} else {
					grant = cleaned
				}
			}
		}
	}
	
	// Sanitize extension ID to prevent XSS and path traversal
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	extID = reg.ReplaceAllString(extID, "")

	w.Header().Set("Content-Type", "application/json")

	if grant == "" || extID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"error":"Missing Parameters","message":"Both grant/code and state (extension ID) are required."}`))
		return
	}

	gb.SetExtensionSessionGrantByID(extID, grant)
	_, err := gb.InvokeExtensionActionJSON(extID, "completeGrant")
	if err != nil {
		// Fall back to oauth code if not session grant
		gb.SetExtensionAuthCodeByID(extID, grant)
		_, err = gb.InvokeExtensionActionJSON(extID, "completeSpotifyLogin")
	}

	if err != nil {
		log.Printf("[Authentication Failed] Extension '%s' authentication failed: %v", extID, err)
		w.WriteHeader(http.StatusInternalServerError)
		resBytes, _ := json.Marshal(map[string]interface{}{
			"success": false,
			"error":   "Authentication Failed",
			"message": err.Error(),
		})
		_, _ = w.Write(resBytes)
		return
	}

	log.Printf("[Authentication Success] Extension '%s' successfully authenticated and session keys registered.", extID)
	w.WriteHeader(http.StatusOK)
	resBytes, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Successfully verified and loaded session keys for extension '%s'.", extID),
	})
	_, _ = w.Write(resBytes)
}

