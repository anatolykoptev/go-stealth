# Browser identity generator

`GenerateIdentity(seed, Constraints)` draws a `BrowserIdentity` from a model of
real browser traffic instead of always presenting one canonical browser.

What the model decides: which built-in entry is presented (browser, major
version, OS, desktop or mobile), in proportion to its share of observed
traffic. What it does not decide: any header text. The User-Agent, Client Hints
and header values come from the built-in entry, so the values measured against
real captures stay authoritative, and every identity is backed by a TLS profile
the library actually emulates. Versions the model has but the library has not
measured are left out and the rest renormalised, so the model can only choose
among identities the library can present coherently.

- `Constraints.Profiles` is the available TLS profile set (default: the
  profiles of `BuiltinProfiles`; legacy profiles only when listed).
- `Constraints.Locales` picks the accept-language; the model does not cover it.
- `Constraints.MinWeight` drops identities the model gives less than 1% (default; negative turns it off) and renormalises, so a small fleet does not draw the long tail.
- `IdentityDistribution` returns the exact probability of each identity, for
  callers that choose by their own stable scheme.
- The same seed gives the same identity under one model version
  (`GeneratedIdentity.ModelVersion`). A new model or profile set can change the
  answer for a seed, so store the identity you drew, not the seed.

Data and licence: see [NOTICE](../NOTICE) and
[internal/identitymodel/testdata/README.md](../internal/identitymodel/testdata/README.md).

The model file is refreshed monthly by a workflow that opens a pull request after a freshness check; it never merges.
