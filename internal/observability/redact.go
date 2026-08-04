package observability

import (
	"context"
	"log/slog"
	"strings"
)

const redacted = "[REDACTED]"

type RedactingHandler struct {
	next slog.Handler
}

func NewRedactingHandler(next slog.Handler) *RedactingHandler {
	return &RedactingHandler{next: next}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	copy := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		copy.AddAttrs(redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, copy)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redactedAttrs := make([]slog.Attr, len(attrs))
	for index, attr := range attrs {
		redactedAttrs[index] = redactAttr(attr)
	}
	return &RedactingHandler{next: h.next.WithAttrs(redactedAttrs)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if isSensitiveKey(attr.Key) {
		return slog.String(attr.Key, redacted)
	}
	if attr.Value.Kind() == slog.KindGroup {
		children := attr.Value.Group()
		for index := range children {
			children[index] = redactAttr(children[index])
		}
		return slog.Group(attr.Key, childrenToAny(children)...)
	}
	return attr
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, fragment := range []string{"authorization", "token", "password", "database_url", "file_url", "email", "owner"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func childrenToAny(children []slog.Attr) []any {
	values := make([]any, len(children))
	for index := range children {
		values[index] = children[index]
	}
	return values
}
