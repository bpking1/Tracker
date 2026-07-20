package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"traker/internal/domain"
)

func TestUpdateOnlyChangesTargetLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traker.txt")
	original := "# header\r\n\r\n- 第一部 2026.07.01\r\nunknown line stays exact\r\nx 第三部 2026.07.03 2026.07.01 *5\r\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil { t.Fatal(err) }
	store, err := New(path); if err != nil { t.Fatal(err) }
	snapshot, _ := store.Read()
	record := snapshot.Records[0]
	comment := "已修改"
	input := domain.RecordInput{Status: record.Status, Title: record.Title, CreatedAt: record.CreatedAt, Comment: &comment}
	if _, err := store.Update(snapshot.Revision, record.Key, input); err != nil { t.Fatal(err) }
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "unknown line stays exact\r\nx 第三部 2026.07.03 2026.07.01 *5\r\n") { t.Fatalf("unrelated lines changed:\n%s", data) }
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(path), "backups")); if len(entries) != 1 { t.Fatalf("expected one backup, got %d", len(entries)) }
}

func TestRevisionConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traker.txt")
	if err := os.WriteFile(path, []byte("- 原记录\n"), 0o644); err != nil { t.Fatal(err) }
	store, _ := New(path); snapshot, _ := store.Read()
	if err := os.WriteFile(path, []byte("- 外部修改\n"), 0o644); err != nil { t.Fatal(err) }
	_, err := store.Add(snapshot.Revision, domain.RecordInput{Status: domain.Planned, Title: "不能覆盖"})
	if !errors.Is(err, ErrConflict) { t.Fatalf("expected conflict, got %v", err) }
	data, _ := os.ReadFile(path); if string(data) != "- 外部修改\n" { t.Fatal("external change was overwritten") }
}

func TestDuplicateLinesHaveDistinctKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traker.txt")
	_ = os.WriteFile(path, []byte("- 相同\n- 相同\n"), 0o644)
	store, _ := New(path); snapshot, _ := store.Read()
	if len(snapshot.Records) != 2 || snapshot.Records[0].Key == snapshot.Records[1].Key { t.Fatal("duplicate records must have distinct keys") }
}

