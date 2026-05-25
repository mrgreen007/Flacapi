package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		ItemID:         itemID,
		Status:         "finalizing",
		CoverArtFailed: true,
		CreatedAt:      time.Now(),
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
	if res["cover_art_failed"] != true {
		t.Errorf("Expected cover_art_failed true, got %v", res["cover_art_failed"])
	}
}

func TestHandleProgressEndpointsNormalization(t *testing.T) {
	itemID := "test-norm-item"

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

	// 2. Test Get All Download Progress
	{
		req := httptest.NewRequest("GET", "/api/v1/download/progress/all", nil)
		w := httptest.NewRecorder()
		HandleGetAllDownloadProgress(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /progress/all failed: %d", resp.StatusCode)
		}

		var res struct {
			Items map[string]map[string]interface{} `json:"items"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)

		item, exists := res.Items[itemID]
		if !exists {
			t.Fatalf("GET /progress/all: item %s not found in response", itemID)
		}
		if p, ok := item["progress"].(float64); !ok || p != 45.5 {
			t.Errorf("GET /progress/all: expected progress 45.5, got %v", item["progress"])
		}

		// Verify fields are omitted
		if _, exists := item["bytes_total"]; exists {
			t.Error("GET /progress/all: expected bytes_total to be omitted")
		}
		if _, exists := item["bytes_received"]; exists {
			t.Error("GET /progress/all: expected bytes_received to be omitted")
		}
		if _, exists := item["speed_mbps"]; exists {
			t.Error("GET /progress/all: expected speed_mbps to be omitted")
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

