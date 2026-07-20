package parser

import (
	"reflect"
	"testing"

	"traker/internal/domain"
)

func TestParseCompleteRecord(t *testing.T) {
	record, ok := ParseLine(`x 心灵奇旅 tm:508442 2026.07.20 2026.07.01 *5 ~S01E02 #动画 #皮克斯 | 第二次看\|依然很好`, 3)
	if !ok { t.Fatal("expected record") }
	if record.Status != domain.Watched || record.Title != "心灵奇旅" { t.Fatalf("unexpected record: %#v", record) }
	if record.MediaRef == nil || record.MediaRef.Type != "tm" || record.MediaRef.ID != 508442 { t.Fatalf("unexpected media ref: %#v", record.MediaRef) }
	if value(record.CompletedAt) != "2026.07.20" || value(record.CreatedAt) != "2026.07.01" { t.Fatal("dates were parsed incorrectly") }
	if value(record.Rating) != 5 || value(record.Progress) != "S01E02" { t.Fatal("rating or progress was parsed incorrectly") }
	if !reflect.DeepEqual(record.Tags, []string{"动画", "皮克斯"}) || value(record.Comment) != "第二次看|依然很好" { t.Fatal("tags or comment were parsed incorrectly") }
}

func TestCommentsStopMetadataParsing(t *testing.T) {
	record, _ := ParseLine(`- 标题 2026.07.01 | #不是标签 *9 tm:42`, 1)
	if len(record.Tags) != 0 || record.Rating != nil || record.MediaRef != nil { t.Fatal("comment content was parsed as metadata") }
	if value(record.Comment) != "#不是标签 *9 tm:42" { t.Fatalf("unexpected comment %q", value(record.Comment)) }
}

func TestInvalidDatesArePreservedInRawLine(t *testing.T) {
	raw := `> 黑镜 2026.07.01 2026.07.02 ~S03E02`
	record, _ := ParseLine(raw, 1)
	if record.RawLine != raw || len(record.Warnings) == 0 { t.Fatal("expected raw line and warning") }
	if record.CreatedAt != nil || record.CompletedAt != nil { t.Fatal("ambiguous dates must not be guessed") }
}

func TestEscapedMetadataInTitle(t *testing.T) {
	record, _ := ParseLine(`- 电影 \*5 \#标题 \~尾声`, 1)
	if record.Title != "电影 *5 #标题 ~尾声" { t.Fatalf("unexpected title %q", record.Title) }
	if record.Rating != nil || len(record.Tags) != 0 { t.Fatal("escaped title was parsed as metadata") }
}

func TestSerializeAndParse(t *testing.T) {
	created, completed, rating, progress, comment := "2026.07.01", "2026.07.20", 4, "S02E03", "不错 | 推荐"
	input := domain.RecordInput{Status: domain.Watched, Title: "标题 *5", MediaRef: &domain.MediaRef{Type: "tv", ID: 42}, CompletedAt: &completed, CreatedAt: &created, Rating: &rating, Progress: &progress, Tags: []string{"科幻"}, Comment: &comment}
	line, err := Serialize(input); if err != nil { t.Fatal(err) }
	record, _ := ParseLine(line, 1)
	if record.Title != input.Title || value(record.Comment) != comment || value(record.Rating) != rating { t.Fatalf("round trip failed: %s %#v", line, record) }
}

func TestCommentsAndBlankLinesAreIgnored(t *testing.T) {
	for _, line := range []string{"", "   ", "# Traker", "  # note"} { if _, ok := ParseLine(line, 1); ok { t.Fatalf("expected %q to be ignored", line) } }
}

func value[T any](pointer *T) T { var zero T; if pointer == nil { return zero }; return *pointer }

