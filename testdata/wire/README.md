# testdata/wire

Real-browser captures used by `fingerprint_wire_live_test.go` (`make fingerprint-wire`).
Each file records its browser, exact version, build, platform, headless flag,
capture time, endpoint and mode. All were taken through `https://tls.peet.ws/api/all`,
the service the check measures go-stealth with; the client IP and TCP fields of the
raw responses are not kept.

| file | browser | use |
|---|---|---|
| `google_chrome_156_nav.json` | Google Chrome 156.0.8078.80 (stable, branded), first connection | oracle for chrome_156, the default and chrome_152 |
| `google_chrome_156_fetch.json` | same Chrome, page `fetch()`, a later connection | HTTP/2 priority and `priority` header of fetch mode; its TLS part includes pre_shared_key from session resumption, so it is information only |
| `cft_131_nav.json` | Chrome for Testing 131.0.6778.264 | information for chrome_131, which is tls-client's contributed profile and intentionally differs |
| `cft_152_nav.json` | Chrome for Testing 152.0.7977.82 | information: CfT sends one more extension than branded Chrome |

Chrome for Testing is not branded Chrome: its JA4 and its header order (accept-language
placed after sec-ch-ua-platform) differ from stable Google Chrome's, so a profile that
claims Chrome stable is compared with the branded capture.
