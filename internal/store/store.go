package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"traker/internal/domain"
	"traker/internal/parser"
)

var ErrConflict = errors.New("file revision conflict")
var ErrNotFound = errors.New("record not found")

type Store struct {
	path string
	mu   sync.Mutex
}

type line struct {
	content string
	ending  string
}

func New(path string) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil { return nil, err }
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil { return nil, err }
	if _, err := os.Stat(absolute); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(absolute, []byte("# Traker\n\n"), 0o644); err != nil { return nil, err }
	} else if err != nil { return nil, err }
	return &Store{path: absolute}, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Read() (domain.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readUnlocked()
}

func (s *Store) Add(revision string, input domain.RecordInput) (domain.Snapshot, error) {
	serialized, err := parser.Serialize(input)
	if err != nil { return domain.Snapshot{}, err }
	return s.mutate(revision, func(lines []line, _ domain.Snapshot) ([]line, error) {
		ending := dominantEnding(lines)
		if len(lines) > 0 && lines[len(lines)-1].ending == "" {
			lines[len(lines)-1].ending = ending
		}
		return append(lines, line{content: serialized, ending: ending}), nil
	})
}

func (s *Store) Update(revision, key string, input domain.RecordInput) (domain.Snapshot, error) {
	serialized, err := parser.Serialize(input)
	if err != nil { return domain.Snapshot{}, err }
	return s.mutate(revision, func(lines []line, snapshot domain.Snapshot) ([]line, error) {
		lineNumber, ok := findLine(snapshot, key)
		if !ok { return nil, ErrNotFound }
		lines[lineNumber-1].content = serialized
		return lines, nil
	})
}

func (s *Store) Delete(revision, key string) (domain.Snapshot, error) {
	return s.mutate(revision, func(lines []line, snapshot domain.Snapshot) ([]line, error) {
		lineNumber, ok := findLine(snapshot, key)
		if !ok { return nil, ErrNotFound }
		index := lineNumber - 1
		if index > 0 && lines[index].ending == "" { lines[index-1].ending = "" }
		return append(lines[:index], lines[index+1:]...), nil
	})
}

func (s *Store) Match(revision, key, mediaType string, id int) (domain.Snapshot, error) {
	if (mediaType != "tm" && mediaType != "tv") || id <= 0 { return domain.Snapshot{}, fmt.Errorf("invalid media reference") }
	return s.mutate(revision, func(lines []line, snapshot domain.Snapshot) ([]line, error) {
		lineNumber, ok := findLine(snapshot, key)
		if !ok { return nil, ErrNotFound }
		var target domain.Record
		for _, record := range snapshot.Records { if record.Key == key { target = record; break } }
		input := parser.InputFromRecord(target)
		input.MediaRef = &domain.MediaRef{Type: mediaType, ID: id}
		serialized, err := parser.Serialize(input)
		if err != nil { return nil, err }
		lines[lineNumber-1].content = serialized
		return lines, nil
	})
}

func (s *Store) mutate(expected string, mutate func([]line, domain.Snapshot) ([]line, error)) (domain.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil { return domain.Snapshot{}, err }
	snapshot := parseSnapshot(data)
	if expected == "" || expected != snapshot.Revision { return snapshot, ErrConflict }
	lines := splitLines(string(data))
	lines, err = mutate(lines, snapshot)
	if err != nil { return domain.Snapshot{}, err }
	if err := s.writeAtomic([]byte(joinLines(lines)), data); err != nil { return domain.Snapshot{}, err }
	return s.readUnlocked()
}

func (s *Store) readUnlocked() (domain.Snapshot, error) {
	data, err := os.ReadFile(s.path)
	if err != nil { return domain.Snapshot{}, err }
	return parseSnapshot(data), nil
}

func parseSnapshot(data []byte) domain.Snapshot {
	revision := revisionOf(data)
	snapshot := domain.Snapshot{Revision: revision, Records: []domain.Record{}, FileWarnings: []domain.ParseWarning{}}
	occurrences := map[string]int{}
	for index, item := range splitLines(string(data)) {
		record, ok := parser.ParseLine(item.content, index+1)
		if !ok { continue }
		lineHash := revisionOf([]byte(item.content))
		occurrences[lineHash]++
		record.Key = keyFor(revision, lineHash, occurrences[lineHash])
		snapshot.Records = append(snapshot.Records, *record)
	}
	return snapshot
}

func (s *Store) writeAtomic(next, previous []byte) error {
	directory := filepath.Dir(s.path)
	temp, err := os.CreateTemp(directory, ".traker-*.tmp")
	if err != nil { return err }
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err = temp.Write(next); err != nil { temp.Close(); return err }
	if err = temp.Sync(); err != nil { temp.Close(); return err }
	if err = temp.Close(); err != nil { return err }

	backupDir := filepath.Join(directory, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil { return err }
	backupName := fmt.Sprintf("traker-%s-%s.txt", time.Now().Format("20060102-150405.000"), revisionOf(previous)[:8])
	if err := writeSynced(filepath.Join(backupDir, backupName), previous); err != nil { return err }
	if err := replaceFile(tempPath, s.path); err != nil { return err }
	pruneBackups(backupDir, 20)
	return nil
}

func writeSynced(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil { return err }
	if _, err = file.Write(data); err != nil { file.Close(); return err }
	if err = file.Sync(); err != nil { file.Close(); return err }
	return file.Close()
}

func pruneBackups(directory string, keep int) {
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) <= keep { return }
	sort.Slice(entries, func(i, j int) bool { ai, _ := entries[i].Info(); aj, _ := entries[j].Info(); return ai.ModTime().Before(aj.ModTime()) })
	for _, entry := range entries[:len(entries)-keep] { _ = os.Remove(filepath.Join(directory, entry.Name())) }
}

func splitLines(value string) []line {
	if value == "" { return nil }
	var result []line
	for len(value) > 0 {
		index := strings.IndexByte(value, '\n')
		if index < 0 { result = append(result, line{content: value}); break }
		content, ending := value[:index], "\n"
		if strings.HasSuffix(content, "\r") { content, ending = strings.TrimSuffix(content, "\r"), "\r\n" }
		result = append(result, line{content: content, ending: ending})
		value = value[index+1:]
	}
	return result
}

func joinLines(lines []line) string { var b strings.Builder; for _, item := range lines { b.WriteString(item.content); b.WriteString(item.ending) }; return b.String() }
func dominantEnding(lines []line) string { crlf, lf := 0, 0; for _, item := range lines { if item.ending == "\r\n" { crlf++ } else if item.ending == "\n" { lf++ } }; if crlf > lf { return "\r\n" }; return "\n" }
func revisionOf(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func keyFor(revision, lineHash string, occurrence int) string { sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", revision, lineHash, occurrence))); return hex.EncodeToString(sum[:12]) }
func findLine(snapshot domain.Snapshot, key string) (int, bool) { for _, record := range snapshot.Records { if record.Key == key { return record.LineNumber, true } }; return 0, false }

func copyFile(source, destination string) error {
	in, err := os.Open(source); if err != nil { return err }; defer in.Close()
	out, err := os.Create(destination); if err != nil { return err }
	_, copyErr := io.Copy(out, in); syncErr := out.Sync(); closeErr := out.Close()
	if copyErr != nil { return copyErr }; if syncErr != nil { return syncErr }; return closeErr
}

