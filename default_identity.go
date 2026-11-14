package stealth

// DefaultProfile returns the TLS profile NewClient uses when no WithProfile or
// WithIdentity is given. It is the value to compare against or log, instead of
// hard-coding a ProfileChrome* constant that stops matching the client when
// the default moves.
func DefaultProfile() TLSProfile {
	return defaultConfig().profile
}

// DefaultIdentity returns the BrowserIdentity of a client built with no
// WithProfile or WithIdentity: the default TLS profile, the first
// BuiltinProfiles User-Agent for it (the Windows desktop variant) and the
// Client Hints derived from that User-Agent. It needs no client and opens no
// connection, so it suits callers that only need the matching headers, for
// example a plain net/http request or a CDP session; a caller that holds a
// BrowserClient should use its Identity() instead, which also covers
// WithProfile and WithIdentity.
//
// The identity changes when the default profile changes (a minor release), so
// code that persists a User-Agent must store the string, not the call.
func DefaultIdentity() BrowserIdentity {
	return resolveIdentity(defaultConfig())
}
