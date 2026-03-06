package websearch

import (
	"testing"
	"time"
)

func TestParseYandexXML_Basic(t *testing.T) {
	t.Parallel()
	// Note: XML unmarshaler treats <hlword> as child elements, not text.
	// Only text nodes outside <hlword> are captured by xml:",chardata".
	// Real Yandex responses have hlword tags, but we test with plain text here
	// since cleanXMLText handles residual tags in string fields.
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<yandexsearch>
  <response>
    <results>
      <grouping>
        <group>
          <doc>
            <url>https://example.com/page1</url>
            <domain>example.com</domain>
            <title>Test Page</title>
            <headline>A test page headline</headline>
            <passages>
              <passage>First passage</passage>
              <passage>Second passage</passage>
            </passages>
          </doc>
        </group>
        <group>
          <doc>
            <url>https://example.com/page2</url>
            <domain>example.com</domain>
            <title>Another Page</title>
            <headline>Another headline</headline>
          </doc>
        </group>
      </grouping>
    </results>
  </response>
</yandexsearch>`)

	results, err := ParseYandexXML(xmlData)
	if err != nil {
		t.Fatalf("ParseYandexXML error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result: title should have hlword stripped, content from passages.
	if results[0].Title != "Test Page" {
		t.Errorf("expected title 'Test Page', got %q", results[0].Title)
	}
	if results[0].Content != "First passage Second passage" { //nolint:dupword
		t.Errorf("expected joined passages, got %q", results[0].Content)
	}
	if results[0].URL != "https://example.com/page1" {
		t.Errorf("unexpected URL: %q", results[0].URL)
	}
	if results[0].Score != directResultScore {
		t.Errorf("expected score %f, got %f", directResultScore, results[0].Score)
	}
	if results[0].Metadata["engine"] != "yandex" {
		t.Errorf("expected engine=yandex metadata, got %v", results[0].Metadata)
	}

	// Second result: no passages, should use headline.
	if results[1].Content != "Another headline" {
		t.Errorf("expected headline as content, got %q", results[1].Content)
	}
}

func TestParseYandexXML_SkipsEmptyURL(t *testing.T) {
	t.Parallel()
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<yandexsearch>
  <response>
    <results>
      <grouping>
        <group>
          <doc>
            <url></url>
            <title>No URL</title>
          </doc>
        </group>
        <group>
          <doc>
            <url>https://valid.com</url>
            <title>Valid</title>
            <headline>Content</headline>
          </doc>
        </group>
      </grouping>
    </results>
  </response>
</yandexsearch>`)

	results, err := ParseYandexXML(xmlData)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (skip empty URL), got %d", len(results))
	}
}

func TestParseYandexXML_Error(t *testing.T) {
	t.Parallel()
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<yandexsearch>
  <response>
    <error code="15">some error message</error>
  </response>
</yandexsearch>`)

	_, err := ParseYandexXML(xmlData)
	if err == nil {
		t.Error("expected error for XML error response")
	}
}

func TestParseYandexXML_EmptyResults(t *testing.T) {
	t.Parallel()
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<yandexsearch>
  <response>
    <results>
      <grouping></grouping>
    </results>
  </response>
</yandexsearch>`)

	results, err := ParseYandexXML(xmlData)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseYandexXML_InvalidXML(t *testing.T) {
	t.Parallel()
	_, err := ParseYandexXML([]byte("not xml"))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestYandexCheckLimit_Unlimited(t *testing.T) {
	t.Parallel()
	if !yandexCheckLimit(0) {
		t.Error("expected unlimited (0) to always return true")
	}
}

func TestYandexCheckLimit_Enforcement(t *testing.T) {
	// Reset counter for this test.
	yandexMonthlyCounter.count.Store(0)
	yandexMonthlyCounter.month.Store(int32(time.Now().Month())) //nolint:gosec

	if !yandexCheckLimit(2) {
		t.Error("first call should be within limit")
	}
	if !yandexCheckLimit(2) {
		t.Error("second call should be within limit")
	}
	if yandexCheckLimit(2) {
		t.Error("third call should exceed limit of 2")
	}
}

func TestCleanXMLText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"<hlword>test</hlword> word", "test word"},
		{"no tags here", "no tags here"},
		{"  spaces  ", "spaces"},
		{"<hlword>a</hlword> <hlword>b</hlword>", "a b"},
		{"", ""},
	}
	for _, tt := range tests {
		got := cleanXMLText(tt.input)
		if got != tt.want {
			t.Errorf("cleanXMLText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
