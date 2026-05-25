package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gb "github.com/zarz/spotiflac_android/go_backend"
)

func TestSafePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flacapi-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save original and restore at end
	originalDataDir := DataDir
	defer func() { DataDir = originalDataDir }()

	DataDir = tempDir

	// Test valid relative path
	p, err := SafePath("song.flac")
	if err != nil {
		t.Errorf("Unexpected error for valid relative path: %v", err)
	}
	expected := filepath.Join(tempDir, "song.flac")
	absP, _ := filepath.Abs(p)
	absExpected, _ := filepath.Abs(expected)
	if absP != absExpected {
		t.Errorf("Expected path %s, got %s", absExpected, absP)
	}

	// Test valid absolute path inside boundary
	p2, err := SafePath(filepath.Join(tempDir, "subdir", "song.flac"))
	if err != nil {
		t.Errorf("Unexpected error for valid absolute path: %v", err)
	}
	expected2 := filepath.Join(tempDir, "subdir", "song.flac")
	absP2, _ := filepath.Abs(p2)
	absExpected2, _ := filepath.Abs(expected2)
	if absP2 != absExpected2 {
		t.Errorf("Expected path %s, got %s", absExpected2, absP2)
	}

	// Test path traversal rejection (relative)
	_, err = SafePath("../outside.flac")
	if err == nil {
		t.Error("Expected error for relative path traversal, but got none")
	}

	// Test path traversal rejection (absolute)
	_, err = SafePath(filepath.Join(tempDir, "..", "outside.flac"))
	if err == nil {
		t.Error("Expected error for absolute path traversal, but got none")
	}
}

func TestHandleInitItemProgress(t *testing.T) {
	reqBody := `{"itemId":"test-item"}`
	req := httptest.NewRequest("POST", "/api/v1/download/item/init", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	HandleInitItemProgress(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if res["success"] != true {
		t.Errorf("Expected success to be true, got %v", res["success"])
	}
}

func TestHandleGetDownloadProgressFiltered(t *testing.T) {
	itemID := "test-progress-item"
	DownloadStates.Store(itemID, &DownloadState{
		ItemID:    itemID,
		Status:    "finalizing",
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/v1/download/progress?itemId="+itemID, nil)
	w := httptest.NewRecorder()

	HandleGetDownloadProgress(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if res["item_id"] != itemID {
		t.Errorf("Expected item_id %s, got %v", itemID, res["item_id"])
	}
	if res["status"] != "finalizing" {
		t.Errorf("Expected status 'finalizing', got %v", res["status"])
	}
}

func TestHandleProgressEndpointsNormalization(t *testing.T) {
	itemID := "test-norm-item"

	DownloadStates.Store(itemID, &DownloadState{
		ItemID: itemID,
		Status: "downloading",
	})
	defer DownloadStates.Delete(itemID)

	gb.InitItemProgress(itemID)
	gb.SetItemProgress(itemID, 0.455342, 455342, 1000000)
	gb.SetItemBytesReceivedWithSpeed(itemID, 455342, 2.436)

	// Clean up after test
	defer gb.ClearItemProgress(itemID)

	// 1. Test Get Single Progress
	{
		req := httptest.NewRequest("GET", "/api/v1/download/progress?itemId="+itemID, nil)
		w := httptest.NewRecorder()
		HandleGetDownloadProgress(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /progress failed: %d", resp.StatusCode)
		}

		var res map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&res)

		if p, ok := res["progress"].(float64); !ok || p != 45.5 {
			t.Errorf("GET /progress: expected progress 45.5, got %v", res["progress"])
		}

		// Verify fields are omitted
		if _, exists := res["bytes_total"]; exists {
			t.Error("GET /progress: expected bytes_total to be omitted")
		}
		if _, exists := res["bytes_received"]; exists {
			t.Error("GET /progress: expected bytes_received to be omitted")
		}
		if _, exists := res["speed_mbps"]; exists {
			t.Error("GET /progress: expected speed_mbps to be omitted")
		}
	}



	// 3. Test Get Delta Download Progress
	{
		req := httptest.NewRequest("GET", "/api/v1/download/progress/delta?since=0", nil)
		w := httptest.NewRecorder()
		HandleGetAllDownloadProgressDelta(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /progress/delta failed: %d", resp.StatusCode)
		}

		var res struct {
			Items map[string]map[string]interface{} `json:"items"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)

		item, exists := res.Items[itemID]
		if !exists {
			t.Fatalf("GET /progress/delta: item %s not found in response", itemID)
		}
		if p, ok := item["progress"].(float64); !ok || p != 45.5 {
			t.Errorf("GET /progress/delta: expected progress 45.5, got %v", item["progress"])
		}

		// Verify fields are omitted
		if _, exists := item["bytes_total"]; exists {
			t.Error("GET /progress/delta: expected bytes_total to be omitted")
		}
		if _, exists := item["bytes_received"]; exists {
			t.Error("GET /progress/delta: expected bytes_received to be omitted")
		}
		if _, exists := item["speed_mbps"]; exists {
			t.Error("GET /progress/delta: expected speed_mbps to be omitted")
		}
	}
}

func TestDownloadStatesPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flacapi-persist-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDataDir := DataDir
	defer func() { DataDir = originalDataDir }()
	DataDir = tempDir

	// Ensure the sync.Map is empty initially
	DownloadStates.Range(func(key, value interface{}) bool {
		DownloadStates.Delete(key)
		return true
	})

	now := time.Now().Round(time.Second)

	// Create three sample states
	// 1. Completed state
	DownloadStates.Store("item-completed", &DownloadState{
		ItemID:       "item-completed",
		Status:       "completed",
		FilePath:     filepath.Join(tempDir, "song.m4a"),
		CreatedAt:    now,
		LastAccessed: now,
	})
	// 2. Failed state
	DownloadStates.Store("item-failed", &DownloadState{
		ItemID:       "item-failed",
		Status:       "failed",
		Error:        "some download error",
		CreatedAt:    now,
		LastAccessed: now,
	})
	// 3. Active/In-progress state (should be marked failed on load)
	DownloadStates.Store("item-active", &DownloadState{
		ItemID:       "item-active",
		Status:       "downloading",
		CreatedAt:    now,
		LastAccessed: now,
	})

	// Save to disk
	saveDownloadStates()

	// Verify states file exists
	filePath := filepath.Join(tempDir, "download_states.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("download_states.json was not created on disk")
	}

	// Wipe states from memory
	DownloadStates.Range(func(key, value interface{}) bool {
		DownloadStates.Delete(key)
		return true
	})

	// Load states back from disk
	LoadDownloadStates()

	// 1. Assert Completed state is restored correctly
	if val, ok := DownloadStates.Load("item-completed"); !ok {
		t.Error("Failed to restore completed task")
	} else {
		s := val.(*DownloadState)
		if s.Status != "completed" || s.FilePath != filepath.Join(tempDir, "song.m4a") {
			t.Errorf("Completed task restored incorrectly: %+v", s)
		}
	}

	// 2. Assert Failed state is restored correctly
	if val, ok := DownloadStates.Load("item-failed"); !ok {
		t.Error("Failed to restore failed task")
	} else {
		s := val.(*DownloadState)
		if s.Status != "failed" || s.Error != "some download error" {
			t.Errorf("Failed task restored incorrectly: %+v", s)
		}
	}

	// 3. Assert Active state is transitioned to failed with interruption message
	if val, ok := DownloadStates.Load("item-active"); !ok {
		t.Error("Failed to restore active task")
	} else {
		s := val.(*DownloadState)
		if s.Status != "failed" || !strings.Contains(s.Error, "interrupted") {
			t.Errorf("Active task was not correctly marked as failed: %+v", s)
		}
	}
}



