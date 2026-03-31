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
