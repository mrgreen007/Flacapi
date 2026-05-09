package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
