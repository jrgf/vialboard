package httpapi_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type operationResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	StatusURL   string `json:"status_url"`
	Progress    int    `json:"progress"`
	Attempt     int    `json:"attempt"`
	MaxAttempts int    `json:"max_attempts"`
	Result      *struct {
		DownloadURL string `json:"download_url"`
		Filename    string `json:"filename"`
		Rows        int    `json:"rows"`
	} `json:"result"`
}

func testIssueExport(t *testing.T, serverURL, token, otherToken string, issue issueResponse) string {
	t.Helper()
	submit := func() operationResponse {
		req, err := http.NewRequest(http.MethodPost, serverURL+"/exports/issues", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", "issue-export-integration")
		req.Header.Set("Prefer", "respond-async")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusAccepted || response.Header.Get("Location") == "" {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("export submission status = %d, body = %s", response.StatusCode, body)
		}
		var operation operationResponse
		if err := json.NewDecoder(response.Body).Decode(&operation); err != nil {
			t.Fatal(err)
		}
		return operation
	}

	operation := submit()
	duplicate := submit()
	if operation.ID == "" || operation.StatusURL == "" || duplicate.ID != operation.ID {
		t.Fatalf("idempotent export operations = %+v, %+v", operation, duplicate)
	}
	statusURL := serverURL + operation.StatusURL
	if status, _ := requestJSON(t, statusURL, http.MethodGet, nil); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated operation status = %d", status)
	}
	if status, _ := requestJSONWithToken(t, statusURL, http.MethodGet, nil, otherToken); status != http.StatusNotFound {
		t.Fatalf("other owner operation status = %d", status)
	}

	eventRequest, err := http.NewRequest(http.MethodGet, statusURL+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	eventRequest.Header.Set("Authorization", "Bearer "+token)
	eventResponse, err := http.DefaultClient.Do(eventRequest)
	if err != nil {
		t.Fatal(err)
	}
	if eventResponse.StatusCode != http.StatusOK || !strings.Contains(eventResponse.Header.Get("Content-Type"), "text/event-stream") {
		_ = eventResponse.Body.Close()
		t.Fatalf("operation events status = %d, type = %q", eventResponse.StatusCode, eventResponse.Header.Get("Content-Type"))
	}
	events := make(chan []byte, 1)
	go func() {
		body, _ := io.ReadAll(eventResponse.Body)
		_ = eventResponse.Body.Close()
		events <- body
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !slices.Contains([]string{"succeeded", "failed", "cancelled"}, operation.Status) && time.Now().Before(deadline) {
		status, body := requestJSONWithToken(t, statusURL, http.MethodGet, nil, token)
		if status != http.StatusOK {
			t.Fatalf("operation poll status = %d, body = %s", status, body)
		}
		if err := json.Unmarshal(body, &operation); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains([]string{"succeeded", "failed", "cancelled"}, operation.Status) {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if operation.Status != "succeeded" || operation.Progress != 100 || operation.MaxAttempts != 3 || operation.Result == nil || operation.Result.Rows < 1 {
		t.Fatalf("completed export operation = %+v", operation)
	}
	select {
	case body := <-events:
		if !bytes.Contains(body, []byte("event: completion")) || !bytes.Contains(body, []byte(operation.ID)) {
			t.Fatalf("unexpected operation events: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("operation completion event timed out")
	}

	status, body := requestJSONWithToken(t, serverURL+operation.Result.DownloadURL, http.MethodGet, nil, token)
	if status != http.StatusOK {
		t.Fatalf("export download status = %d, body = %s", status, body)
	}
	rows, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 || rows[0][0] != "id" || rows[1][0] != strconv.FormatUint(issue.ID, 10) || rows[1][1] != issue.Title {
		t.Fatalf("unexpected export CSV: %#v", rows)
	}
	if status, _ := requestJSONWithToken(t, statusURL, http.MethodDelete, nil, token); status != http.StatusNoContent {
		t.Fatalf("completed operation cancellation status = %d", status)
	}
	return operation.ID
}
