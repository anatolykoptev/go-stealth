package webtext

import "testing"

func TestCleanHTML_StripsTags(t *testing.T) {
	t.Parallel()
	got := CleanHTML("<p>Hello <b>world</b></p>")
	if got != "Hello world" {
		t.Errorf("CleanHTML = %q, want %q", got, "Hello world")
	}
}

func TestCleanHTML_NestedTags(t *testing.T) {
	t.Parallel()
	got := CleanHTML("<div><p>Nested <span>content</span></p></div>")
	if got != "Nested content" {
		t.Errorf("CleanHTML = %q, want %q", got, "Nested content")
	}
}

func TestCleanHTML_EmptyInput(t *testing.T) {
	t.Parallel()
	got := CleanHTML("")
	if got != "" {
		t.Errorf("CleanHTML empty = %q, want %q", got, "")
	}
}

func TestCleanHTML_NoTags(t *testing.T) {
	t.Parallel()
	got := CleanHTML("plain text")
	if got != "plain text" {
		t.Errorf("CleanHTML = %q, want %q", got, "plain text")
	}
}

func TestCleanLines_RemovesBlankLines(t *testing.T) {
	t.Parallel()
	input := "line1\n\n  \n  line2  \n\nline3"
	got := CleanLines(input)
	want := "line1\nline2\nline3"
	if got != want {
		t.Errorf("CleanLines = %q, want %q", got, want)
	}
}

func TestCleanLines_EmptyInput(t *testing.T) {
	t.Parallel()
	got := CleanLines("")
	if got != "" {
		t.Errorf("CleanLines empty = %q, want %q", got, "")
	}
}

func TestCleanLines_SingleLine(t *testing.T) {
	t.Parallel()
	got := CleanLines("  hello  ")
	if got != "hello" {
		t.Errorf("CleanLines = %q, want %q", got, "hello")
	}
}

func TestNormalizeSpaces_Collapses(t *testing.T) {
	t.Parallel()
	got := NormalizeSpaces("hello   world  \t foo")
	want := "hello world foo"
	if got != want {
		t.Errorf("NormalizeSpaces = %q, want %q", got, want)
	}
}

func TestNormalizeSpaces_EmptyInput(t *testing.T) {
	t.Parallel()
	got := NormalizeSpaces("")
	if got != "" {
		t.Errorf("NormalizeSpaces empty = %q, want %q", got, "")
	}
}

func TestNormalizeSpaces_LeadingTrailing(t *testing.T) {
	t.Parallel()
	got := NormalizeSpaces("  hello  ")
	if got != "hello" {
		t.Errorf("NormalizeSpaces = %q, want %q", got, "hello")
	}
}
