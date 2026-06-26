package diagnostics_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
)

func TestExitCodeMapsDocumentedCategories(t *testing.T) {
	tests := []struct {
		category diagnostics.Category
		want     int
	}{
		{category: diagnostics.CategorySuccess, want: 0},
		{category: diagnostics.CategoryInvalidInput, want: 2},
		{category: diagnostics.CategoryMissingConfig, want: 3},
		{category: diagnostics.CategoryInvalidConfig, want: 4},
		{category: diagnostics.CategoryAttachmentError, want: 5},
		{category: diagnostics.CategoryDeliveryFailure, want: 6},
		{category: diagnostics.CategoryInternalError, want: 1},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if got := diagnostics.ExitCode(tt.category); got != tt.want {
				t.Fatalf("ExitCode(%q) = %d, want %d", tt.category, got, tt.want)
			}
		})
	}
}

func TestFromErrorPreservesKnownDiagnostic(t *testing.T) {
	err := diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, "slack", "provider rejected request")

	got := diagnostics.FromError(err)
	if got.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("Category = %q", got.Category)
	}
	if got.Channel != "slack" {
		t.Fatalf("Channel = %q", got.Channel)
	}
	if got.Message != "provider rejected request" {
		t.Fatalf("Message = %q", got.Message)
	}
}

func TestFromErrorMapsUnknownErrorToInternalError(t *testing.T) {
	got := diagnostics.FromError(errors.New("boom"))
	if got.Category != diagnostics.CategoryInternalError {
		t.Fatalf("Category = %q, want %q", got.Category, diagnostics.CategoryInternalError)
	}
}

func TestWriteFailureEmitsOneLineAndReturnsExitCode(t *testing.T) {
	var stderr bytes.Buffer

	code := diagnostics.WriteFailure(&stderr, diagnostics.New(diagnostics.CategoryInvalidInput, "missing required flag --channel"))
	if code != diagnostics.ExitInvalidInput {
		t.Fatalf("exit code = %d, want %d", code, diagnostics.ExitInvalidInput)
	}
	if got := stderr.String(); got != "invalid_input: missing required flag --channel\n" {
		t.Fatalf("stderr = %q", got)
	}
}
