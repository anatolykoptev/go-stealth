//go:build e2e

package stealth_test

import (
	"testing"

	stealth "github.com/anatolykoptev/go-stealth"
)

func TestByparrSolver_E2E_NowSecure(t *testing.T) {
	solver := stealth.NewByparrSolver(stealth.ByparrConfig{
		BaseURL: "http://127.0.0.1:8191",
	})

	// Should have no cached cookie
	if got := solver.GetCookie("nowsecure.nl"); got != "" {
		t.Fatalf("expected empty cache, got %q", got)
	}

	// Solve CF challenge
	cookie, err := solver.Solve("nowsecure.nl", &stealth.CloudflareError{
		Type:       stealth.ChallengeJS,
		StatusCode: 503,
		RayID:      "test",
	})
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}

	t.Logf("Got cookie: %s", cookie)

	if cookie == "" {
		t.Fatal("expected non-empty cookie")
	}

	// Should be cached now
	cached := solver.GetCookie("nowsecure.nl")
	if cached == "" {
		t.Fatal("expected cached cookie after solve")
	}
	if cached != cookie {
		t.Errorf("cached %q != solved %q", cached, cookie)
	}

	t.Log("E2E: Byparr solver works end-to-end")
}
