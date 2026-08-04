package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactingHandler(t *testing.T) {
	var output bytes.Buffer
	handler := NewRedactingHandler(slog.NewJSONHandler(&output, nil))
	record := slog.NewRecord(timeForTest(), slog.LevelInfo, "request", 0)
	record.AddAttrs(
		slog.String("authorization", "Bearer secret"),
		slog.Group("request", slog.String("owner_email", "person@example.com")),
		slog.String("request_id", "safe-id"),
	)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	logged := output.String()
	for _, secret := range []string{"Bearer secret", "person@example.com"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("log contains secret %q", secret)
		}
	}
	if !strings.Contains(logged, "safe-id") {
		t.Fatal("log omitted safe request ID")
	}
}
