package stealth

import (
	"fmt"
	"strings"
)

// ChallengeType identifies the kind of Cloudflare challenge.
type ChallengeType string

const (
	ChallengeJS        ChallengeType = "js_challenge"
	ChallengeTurnstile ChallengeType = "managed_challenge"
	ChallengeBlock     ChallengeType = "block"
)

// CloudflareError indicates a Cloudflare challenge or block was detected.
type CloudflareError struct {
	Type       ChallengeType
	StatusCode int
	RayID      string
}

func (e *CloudflareError) Error() string {
	return fmt.Sprintf("cloudflare %s (HTTP %d, ray %s)", e.Type, e.StatusCode, e.RayID)
}

// DetectCloudflare inspects a Response for Cloudflare challenge markers.
// Returns nil if the response is not a Cloudflare challenge.
func DetectCloudflare(resp *Response) *CloudflareError {
	if resp.StatusCode != 403 && resp.StatusCode != 503 {
		return nil
	}
	server := strings.ToLower(resp.Headers["server"])
	if !strings.Contains(server, "cloudflare") {
		return nil
	}

	body := strings.ToLower(string(resp.Body))
	rayID := resp.Headers["cf-ray"]

	if resp.StatusCode == 503 && strings.Contains(body, "challenge-platform") {
		return &CloudflareError{Type: ChallengeJS, StatusCode: resp.StatusCode, RayID: rayID}
	}

	if strings.Contains(body, "turnstile-wrapper") || strings.Contains(body, "cf-turnstile") {
		return &CloudflareError{Type: ChallengeTurnstile, StatusCode: resp.StatusCode, RayID: rayID}
	}

	if strings.Contains(body, "you have been blocked") || strings.Contains(body, "cf-error-details") {
		return &CloudflareError{Type: ChallengeBlock, StatusCode: resp.StatusCode, RayID: rayID}
	}

	return nil
}

// CloudflareDetectMiddleware inspects responses for Cloudflare challenges.
// If a challenge is detected, it returns a *CloudflareError (use errors.As to extract).
// Non-challenge responses pass through unchanged.
func CloudflareDetectMiddleware(next Handler) Handler {
	return func(req *Request) (*Response, error) {
		resp, err := next(req)
		if err != nil {
			return resp, err
		}
		if cfErr := DetectCloudflare(resp); cfErr != nil {
			return resp, cfErr
		}
		return resp, nil
	}
}
