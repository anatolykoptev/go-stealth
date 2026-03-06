// Package webextract provides goquery-based HTML content extraction.
//
// It removes boilerplate elements (navigation, ads, footers, widgets)
// and extracts the main content from HTML pages.
package webextract

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// reWhitespace collapses runs of whitespace into a single space.
var reWhitespace = regexp.MustCompile(`\s+`)

// removeSelectors are HTML elements stripped before text extraction.
// Combined list from go-engine and go-enriche for maximum coverage.
var removeSelectors = strings.Join([]string{
	// Standard boilerplate.
	"script", "style", "noscript", "iframe", "svg",
	"header", "footer", "nav", "aside",
	// Ads and non-content.
	".advertisement", ".ad", ".sidebar", ".comments",
	".cookie-banner", ".popup", ".modal", ".newsletter-signup",
	".social-share", ".share-buttons",
	// Navigation and metadata (from go-enriche).
	".breadcrumbs", ".breadcrumb", ".tags", ".tag-list",
	".related", ".related-articles", ".related-news",
	".author-info", ".author-bio", ".author-card",
	".subscribe", ".subscription", ".newsletter",
	// Common CMS widget patterns (from go-enriche).
	".widget", ".incut", ".banner",
	// ARIA and HTML5 hidden.
	"[role=navigation]", "[role=banner]", "[role=contentinfo]",
	"[aria-hidden=true]", "[hidden]",
}, ", ")

// contentSelectors are tried in order to find the main content element.
const contentSelectors = "article, main, [role=main], .post-content, .entry-content, .article-body"

// ExtractContent parses HTML, removes boilerplate, and returns clean text.
// maxLen limits output to that many bytes (0 = no limit), truncated at
// a valid UTF-8 boundary.
func ExtractContent(html string, maxLen int) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	// Remove boilerplate elements.
	doc.Find(removeSelectors).Each(func(_ int, s *goquery.Selection) {
		s.Remove()
	})

	// Find main content container.
	content := doc.Find(contentSelectors).First()
	if content.Length() == 0 {
		content = doc.Find("body")
	}

	text := content.Text()
	text = reWhitespace.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	if maxLen > 0 && len(text) > maxLen {
		text = truncateUTF8(text, maxLen)
	}

	return text
}

// truncateUTF8 returns the first n bytes of s, backing up to a valid
// UTF-8 boundary if n falls in the middle of a multi-byte character.
func truncateUTF8(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) {
		if s[n]&0xC0 != 0x80 { // not a continuation byte
			break
		}
		n--
	}
	return s[:n]
}
