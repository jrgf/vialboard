package httpapi

import (
	"bytes"
	"encoding/csv"
	"testing"
	"time"

	"github.com/jrgf/vialboard/internal/domain"
)

func TestWriteIssueCSV(t *testing.T) {
	due := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.FixedZone("test", -6*60*60))
	teamID := "330b7b69-c0d8-4963-8b56-f249cd8af0f8"
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writeIssueCSV(writer, domain.Issue{
		ID: 42, Title: "Comma, newline", Description: "first\nsecond", Status: domain.StatusOpen,
		Priority: domain.PriorityHigh, DueDate: &due, CreatedBy: "owner", TeamID: &teamID,
		CreatedAt: due, UpdatedAt: due.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&output).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 11 || rows[0][1] != "Comma, newline" || rows[0][2] != "first\nsecond" || rows[0][5] != "2026-08-08" {
		t.Fatalf("unexpected CSV row: %#v", rows)
	}
}
