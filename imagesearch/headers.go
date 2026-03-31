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

// androidHeaders returns headers that mimic an Android app request.
// Google returns JSON from /search?tbm=isch when it sees this UA.
// Reference: SearXNG searx/engines/google_images.py
func androidHeaders() map[string]string {
	return map[string]string{
		"user-agent":      "NSTN/3.60.1 (Linux; U; Android 14; en-US; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/125.0.0.0 Mobile Safari/537.36",
		"accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"accept-language": "en-US,en;q=0.9",
	}
}
