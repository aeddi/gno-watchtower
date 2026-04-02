package doctor

import (
	"testing"
)

func TestTruncate_MultibyteUTF8(t *testing.T) {
	// Each character is 3 bytes in UTF-8.
	s := "日本語テスト"
	got := truncate(s, 2)
	if got != "日本..." {
		t.Errorf("truncate(%q, 2) = %q, want %q", s, got, "日本...")
	}
}
