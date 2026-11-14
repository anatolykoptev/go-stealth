package stealth

import "testing"

// DefaultProfile and DefaultIdentity describe what NewClient() presents.
func TestDefaultIdentity_MatchesBareClient(t *testing.T) {
	bc, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	got, want := DefaultIdentity(), bc.Identity()
	if got.TLSProfile != want.TLSProfile || got.UserAgent != want.UserAgent {
		t.Errorf("DefaultIdentity = %s %q, bare client presents %s %q", got.TLSProfile, got.UserAgent, want.TLSProfile, want.UserAgent)
	}
	if DefaultProfile() != want.TLSProfile {
		t.Errorf("DefaultProfile = %s, client profile %s", DefaultProfile(), want.TLSProfile)
	}
	if got.ClientHints["sec-ch-ua"] == "" {
		t.Errorf("DefaultIdentity has no Client Hints: %v", got.ClientHints)
	}
	if ExtractChromeVersion(got.UserAgent) != profileMajor(DefaultProfile()) {
		t.Errorf("UA major %s != profile %s", ExtractChromeVersion(got.UserAgent), DefaultProfile())
	}
}
