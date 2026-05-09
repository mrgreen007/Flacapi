package gobackend

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestLogBufferExportedHelpersAndRedaction(t *testing.T) {
	ClearLogs()
	SetLoggingEnabled(false)
	LogInfo("test", "ignored access_token=secret")
	LogError("test", "Authorization: Bearer secret-token api_key=value")
	if GetLogCount() != 1 {
		t.Fatalf("disabled logging should keep errors only, got %d", GetLogCount())
	}

	SetLoggingEnabled(true)
	defer SetLoggingEnabled(false)
	LogDebug("debug", "client_secret=secret")
	LogWarn("warn", "warning password=secret")
	GoLog("[GoTag] success token=abc")

	var entries []LogEntry
	if err := json.Unmarshal([]byte(GetLogs()), &entries); err != nil {
		t.Fatalf("GetLogs JSON: %v", err)
	}
	if len(entries) < 4 {
		t.Fatalf("expected log entries, got %#v", entries)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Message, "secret-token") || strings.Contains(entry.Message, "api_key=value") || strings.Contains(entry.Message, "password=secret") {
			t.Fatalf("log was not redacted: %#v", entry)
		}
	}

	sinceJSON := GetLogsSince(1)
	if !strings.Contains(sinceJSON, `"next_index"`) || !strings.Contains(sinceJSON, `"logs"`) {
		t.Fatalf("GetLogsSince = %q", sinceJSON)
	}
	if emptyJSON := GetLogsSince(999); !strings.Contains(emptyJSON, `"logs":[]`) {
		t.Fatalf("GetLogsSince empty = %q", emptyJSON)
	}
	if negativeJSON := GetLogsSince(-5); !strings.Contains(negativeJSON, `"logs"`) {
		t.Fatalf("GetLogsSince negative = %q", negativeJSON)
	}

	ClearLogs()
	if GetLogCount() != 0 || GetLogs() != "[]" {
		t.Fatalf("logs were not cleared: count=%d logs=%s", GetLogCount(), GetLogs())
	}
}

func TestProgressItemHelpersAndWriter(t *testing.T) {
	ClearAllItemProgress()
	itemID := "progress-writer"
	StartItemProgress(itemID)
	SetItemBytesTotal(itemID, int64(progressUpdateThreshold*2))
	SetItemBytesReceived(itemID, int64(progressUpdateThreshold))

	progressJSON := GetItemProgress(itemID)
	if !strings.Contains(progressJSON, `"bytes_received":131072`) || !strings.Contains(progressJSON, `"progress":0.5`) {
		t.Fatalf("GetItemProgress = %q", progressJSON)
	}
	if missing := GetItemProgress("missing"); missing != "{}" {
		t.Fatalf("missing progress = %q", missing)
	}

	var out bytes.Buffer
	writer := NewItemProgressWriter(&out, itemID)
	payload := bytes.Repeat([]byte("x"), progressUpdateThreshold+1)
	n, err := writer.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("progress writer = %d/%v", n, err)
	}
	if out.Len() != len(payload) {
		t.Fatalf("writer output length = %d", out.Len())
	}
	if progressJSON = GetItemProgress(itemID); !strings.Contains(progressJSON, `"bytes_received":131073`) {
		t.Fatalf("progress after writer = %q", progressJSON)
	}

	cancelDownload(itemID)
	defer clearDownloadCancel(itemID)
	n, err = writer.Write([]byte("cancelled"))
	if n != 0 || !errors.Is(err, ErrDownloadCancelled) {
		t.Fatalf("cancelled writer = %d/%v", n, err)
	}

	ClearAllItemProgress()
}

func TestRunWithTimeoutBranches(t *testing.T) {
	if _, err := RunWithTimeout(nil, "1 + 1", time.Millisecond); err == nil {
		t.Fatal("expected nil VM error")
	}

	vm := goja.New()
	value, err := RunWithTimeout(vm, "1 + 2", time.Second)
	if err != nil || value.ToInteger() != 3 {
		t.Fatalf("RunWithTimeout success = %v/%v", value, err)
	}

	timeoutVM := goja.New()
	_, err = RunWithTimeoutAndRecover(timeoutVM, "for (;;) {}", 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !IsTimeoutError(&JSExecutionError{Message: "timeout", IsTimeout: true}) {
		t.Fatal("JSExecutionError should be recognized as timeout")
	}
	if IsTimeoutError(errors.New("plain")) {
		t.Fatal("plain error should not be timeout")
	}
	if (&JSExecutionError{Message: "boom"}).Error() != "boom" {
		t.Fatal("JSExecutionError Error mismatch")
	}
}
