package imagesearch

import stealth "github.com/anatolykoptev/go-stealth"

// searchHeaders returns Chrome-like headers without accept-encoding.
// Go's http.Client handles decompression automatically when accept-encoding
// is not explicitly set; setting it manually disables auto-decompression
// and causes regex parsers to fail on compressed responses.
func searchHeaders() map[string]string {
	h := stealth.ChromeHeaders()
	delete(h, "accept-encoding")
	return h
}

// androidHeaders returns headers mimicking the Google Go Android app.
// This UA triggers Google to return ~50 JSON results instead of 10 HTML.
// Exact format from SearXNG searx/engines/google_images.py.
func androidHeaders() map[string]string {
	return map[string]string{
		"user-agent":      "NSTN/3.60.474802233.release Dalvik/2.1.0 (Linux; U; Android 12; US) gzip",
		"accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"accept-language": "en-US,en;q=0.9",
	}
}
