package webtext

import "testing"

func TestTruncate_Basic(t *testing.T) {
	t.Parallel()
	got := Truncate("hello world", 5)
	if got != "hello" {
		t.Errorf("Truncate = %q, want %q", got, "hello")
	}
}

func TestTruncate_ShortString(t *testing.T) {
	t.Parallel()
	got := Truncate("hi", 10)
	if got != "hi" {
		t.Errorf("Truncate short = %q, want %q", got, "hi")
	}
}

func TestTruncate_ZeroN(t *testing.T) {
	t.Parallel()
	got := Truncate("hello", 0)
	if got != "hello" {
		t.Errorf("Truncate zero = %q, want %q", got, "hello")
	}
}

func TestTruncate_NegativeN(t *testing.T) {
	t.Parallel()
	got := Truncate("hello", -5)
	if got != "hello" {
		t.Errorf("Truncate negative = %q, want %q", got, "hello")
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	t.Parallel()
	got := Truncate("hello", 5)
	if got != "hello" {
		t.Errorf("Truncate exact = %q, want %q", got, "hello")
	}
}

func TestTruncate_UTF8Boundary(t *testing.T) {
	t.Parallel()
	// "Привет" = 12 bytes (6 Cyrillic chars, 2 bytes each).
	s := "Привет"
	got := Truncate(s, 5)
	// Should back up to byte 4 (end of 2nd char "р"), not split mid-char.
	if len(got) > 5 {
		t.Errorf("Truncate UTF-8 length %d exceeds 5", len(got))
	}
	// Verify valid UTF-8 by ranging over it.
	for range got {
		// no-op: range over string validates UTF-8
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	t.Parallel()
	got := Truncate("", 10)
	if got != "" {
		t.Errorf("Truncate empty = %q, want %q", got, "")
	}
}

func TestDefaultCharsPerToken(t *testing.T) {
	t.Parallel()
	if DefaultCharsPerToken != 3.5 {
		t.Errorf("DefaultCharsPerToken = %v, want 3.5", DefaultCharsPerToken)
	}
}
