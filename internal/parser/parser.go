package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"traker/internal/domain"
)

var (
	mediaPattern = regexp.MustCompile(`^(tm|tv|tmdb):(\d+)$`)
	ratingPattern = regexp.MustCompile(`^\*(\d+)$`)
	datePattern = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}$`)
)

func ParseLine(raw string, lineNumber int) (*domain.Record, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil, false
	}

	record := &domain.Record{Status: domain.Planned, RawLine: raw, LineNumber: lineNumber, Tags: []string{}, Warnings: []domain.ParseWarning{}}
	body := strings.TrimLeft(raw, " \t")
	if status, ok := parseStatus(body[0]); ok {
		record.Status = status
		if len(body) == 1 || (body[1] != ' ' && body[1] != '\t') {
			record.Warnings = append(record.Warnings, warning("invalid_status_spacing", "状态符号后需要空格"))
			return record, true
		}
		body = strings.TrimSpace(body[1:])
	} else {
		record.Warnings = append(record.Warnings, warning("legacy_status", "旧格式记录缺少状态符号，已按想看读取"))
		body = strings.TrimSpace(body)
	}

	before, comment, hasComment := splitComment(body)
	if hasComment {
		value := unescape(strings.TrimSpace(comment))
		record.Comment = &value
	}

	var titleTokens []string
	var dates []string
	for _, token := range strings.Fields(before) {
		if isEscapedToken(token) {
			titleTokens = append(titleTokens, unescape(token))
			continue
		}
		if match := mediaPattern.FindStringSubmatch(token); match != nil {
			id, _ := strconv.Atoi(match[2])
			kind := match[1]
			if kind == "tmdb" {
				kind = "tm"
				record.Warnings = append(record.Warnings, warning("legacy_tmdb_id", "旧格式 tmdb: ID 已按电影读取"))
			}
			if record.MediaRef != nil {
				record.Warnings = append(record.Warnings, warning("duplicate_media_ref", "一条记录只能包含一个 TMDB ID"))
				titleTokens = append(titleTokens, token)
			} else {
				record.MediaRef = &domain.MediaRef{Type: kind, ID: id}
			}
			continue
		}
		if datePattern.MatchString(token) {
			if _, err := time.Parse("2006.01.02", token); err != nil {
				record.Warnings = append(record.Warnings, warning("invalid_date", fmt.Sprintf("无效日期：%s", token)))
				titleTokens = append(titleTokens, token)
			} else {
				dates = append(dates, token)
			}
			continue
		}
		if match := ratingPattern.FindStringSubmatch(token); match != nil {
			value, _ := strconv.Atoi(match[1])
			if value < 1 || value > 5 {
				record.Warnings = append(record.Warnings, warning("invalid_rating", "评分必须在 1 到 5 之间"))
				titleTokens = append(titleTokens, token)
			} else if record.Rating != nil {
				record.Warnings = append(record.Warnings, warning("duplicate_rating", "一条记录只能包含一个评分"))
				titleTokens = append(titleTokens, token)
			} else {
				record.Rating = &value
			}
			continue
		}
		if strings.HasPrefix(token, "~") && len(token) > 1 {
			value := unescape(token[1:])
			if record.Progress != nil {
				record.Warnings = append(record.Warnings, warning("duplicate_progress", "一条记录只能包含一个进度"))
				titleTokens = append(titleTokens, token)
			} else {
				record.Progress = &value
			}
			continue
		}
		if strings.HasPrefix(token, "#") && len(token) > 1 {
			record.Tags = append(record.Tags, unescape(token[1:]))
			continue
		}
		if strings.HasPrefix(token, "*") || strings.HasPrefix(token, "tm:") || strings.HasPrefix(token, "tv:") || token == "~" || token == "#" {
			record.Warnings = append(record.Warnings, warning("invalid_marker", fmt.Sprintf("无法识别格式标记：%s", token)))
		}
		titleTokens = append(titleTokens, unescape(token))
	}

	applyDates(record, dates)
	record.Title = strings.Join(titleTokens, " ")
	if record.Title == "" {
		record.Warnings = append(record.Warnings, warning("missing_title", "记录缺少标题"))
	}
	return record, true
}

func Serialize(input domain.RecordInput) (string, error) {
	if strings.TrimSpace(input.Title) == "" {
		return "", fmt.Errorf("title is required")
	}
	symbol, ok := statusSymbol(input.Status)
	if !ok {
		return "", fmt.Errorf("invalid status %q", input.Status)
	}
	parts := []string{symbol, escapeTitle(strings.TrimSpace(input.Title))}
	if input.MediaRef != nil {
		if input.MediaRef.ID <= 0 || (input.MediaRef.Type != "tm" && input.MediaRef.Type != "tv") {
			return "", fmt.Errorf("invalid media reference")
		}
		parts = append(parts, fmt.Sprintf("%s:%d", input.MediaRef.Type, input.MediaRef.ID))
	}
	if input.CompletedAt != nil {
		if input.Status != domain.Watched && input.Status != domain.Dropped {
			return "", fmt.Errorf("completedAt is only valid for watched or dropped records")
		}
		if err := validateDate(*input.CompletedAt); err != nil { return "", err }
		parts = append(parts, *input.CompletedAt)
	}
	if input.CreatedAt != nil {
		if err := validateDate(*input.CreatedAt); err != nil { return "", err }
		parts = append(parts, *input.CreatedAt)
	}
	if input.Rating != nil {
		if *input.Rating < 1 || *input.Rating > 5 { return "", fmt.Errorf("rating must be between 1 and 5") }
		parts = append(parts, fmt.Sprintf("*%d", *input.Rating))
	}
	if input.Progress != nil && strings.TrimSpace(*input.Progress) != "" {
		parts = append(parts, "~"+escapeToken(strings.TrimSpace(*input.Progress)))
	}
	for _, tag := range input.Tags {
		if strings.TrimSpace(tag) == "" { continue }
		if strings.ContainsAny(tag, " \t") { return "", fmt.Errorf("tag cannot contain whitespace") }
		parts = append(parts, "#"+escapeToken(tag))
	}
	line := strings.Join(parts, " ")
	if input.Comment != nil && strings.TrimSpace(*input.Comment) != "" {
		line += " | " + escapeComment(strings.TrimSpace(*input.Comment))
	}
	return line, nil
}

func InputFromRecord(record domain.Record) domain.RecordInput {
	return domain.RecordInput{Status: record.Status, Title: record.Title, MediaRef: record.MediaRef, CompletedAt: record.CompletedAt, CreatedAt: record.CreatedAt, Rating: record.Rating, Progress: record.Progress, Tags: record.Tags, Comment: record.Comment}
}

func applyDates(record *domain.Record, dates []string) {
	switch len(dates) {
	case 0:
	case 1:
		record.CreatedAt = &dates[0]
	case 2:
		if record.Status == domain.Watched || record.Status == domain.Dropped {
			record.CompletedAt, record.CreatedAt = &dates[0], &dates[1]
		} else {
			record.Warnings = append(record.Warnings, warning("too_many_dates_for_status", "想看或在看记录不能包含两个日期"))
		}
	default:
		record.Warnings = append(record.Warnings, warning("too_many_dates", "一条记录不能包含三个或更多日期"))
	}
}

func parseStatus(value byte) (domain.Status, bool) {
	switch value { case '-': return domain.Planned, true; case '>': return domain.Watching, true; case 'x': return domain.Watched, true; case '!': return domain.Dropped, true }
	return "", false
}

func statusSymbol(status domain.Status) (string, bool) {
	switch status { case domain.Planned: return "-", true; case domain.Watching: return ">", true; case domain.Watched: return "x", true; case domain.Dropped: return "!", true }
	return "", false
}

func splitComment(value string) (string, string, bool) {
	escaped := false
	for index, char := range value {
		if char == '\\' { escaped = !escaped; continue }
		if char == '|' && !escaped { return strings.TrimSpace(value[:index]), value[index+1:], true }
		escaped = false
	}
	return value, "", false
}

func isEscapedToken(token string) bool { return strings.HasPrefix(token, `\`) }
func escapeTitle(value string) string { return strings.Join(mapTokens(value, escapeToken), " ") }
func mapTokens(value string, mapper func(string) string) []string { values := strings.Fields(value); for i := range values { values[i] = mapper(values[i]) }; return values }
func escapeToken(value string) string { value = strings.ReplaceAll(value, `\`, `\\`); for _, marker := range []string{"|", "#", "*", "~"} { value = strings.ReplaceAll(value, marker, `\`+marker) }; return value }
func escapeComment(value string) string { value = strings.ReplaceAll(value, `\`, `\\`); return strings.ReplaceAll(value, "|", `\|`) }
func unescape(value string) string { var b strings.Builder; escaped := false; for _, r := range value { if escaped { b.WriteRune(r); escaped = false } else if r == '\\' { escaped = true } else { b.WriteRune(r) } }; if escaped { b.WriteRune('\\') }; return b.String() }
func warning(code, message string) domain.ParseWarning { return domain.ParseWarning{Code: code, Message: message} }
func validateDate(value string) error { if !datePattern.MatchString(value) { return fmt.Errorf("date must use YYYY.MM.DD") }; if _, err := time.Parse("2006.01.02", value); err != nil { return fmt.Errorf("invalid date") }; return nil }
