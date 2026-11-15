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
| `microsoft_edge_156_windows_nav.json` | Microsoft Edge 156.0.4314.58 (stable, branded) on Windows, first connection | oracle for edge_156, its Windows entry and its `sec-ch-ua-platform`; also the `navigator.userAgentData` seen by the page |
| `microsoft_edge_156_windows_fetch.json` | same Edge, page `fetch()` with custom headers, a later connection | header order and values of fetch mode; the TLS part carries pre_shared_key, information only |
| `microsoft_edge_156_windows_xhr.json` | same Edge, page `XMLHttpRequest` | values and names of an XHR; its order puts user-agent before sec-ch-ua-mobile, which the fetch does not, so the order is not asserted |
| `microsoft_edge_156_linux_nav.json`, `microsoft_edge_156_linux_fetch.json` | same Edge build on Linux x86_64 under Xvfb | oracle for the Linux entry; equals the Windows capture except User-Agent and `sec-ch-ua-platform` |
| `google_chrome_156_windows_nav.json` | Google Chrome 156.0.8078.80 on the same Windows runner image and harness as the Edge captures | control: its ja4, ja4_r and peetprint equal the macOS capture's, so the Edge differences are Edge's and not the harness's |
| `cft_131_nav.json` | Chrome for Testing 131.0.6778.264 | information for chrome_131, which is tls-client's contributed profile and intentionally differs |
| `cft_152_nav.json` | Chrome for Testing 152.0.7977.82 | information: CfT sends one more extension than branded Chrome |

Chrome for Testing is not branded Chrome: its JA4 and its header order (accept-language
placed after sec-ch-ua-platform) differ from stable Google Chrome's, so a profile that
claims Chrome stable is compared with the branded capture.

## Microsoft Edge 156 captures

Taken 2026-11-15 on GitHub-hosted runners (`windows-2025`, Windows NT 10.0.26100, x64, and
`ubuntu-24.04`, x86_64), headful (the Linux runs under Xvfb), through the same
`https://tls.peet.ws/api/all` as the Chrome captures. Windows used the Edge preinstalled on the
runner image; Linux installed Edge 156.0.4314.58 from Microsoft's apt package
(sha256 `adcf4ce88e3eb276083a561efd0cb06b6801f09dcb6585f6cc23c59730414c3d`), as a downgrade
of the newer Edge on the image. Both report Edge 156.0.4314.58 on Chromium 156.0.8078.80.
A fresh user-data-dir per run; the browser is started with the echo URL on its command line
and the page is read back over the DevTools protocol, with `--no-first-run
--no-default-browser-check` (Linux adds `--no-sandbox --disable-gpu`, which do not touch the
wire). The client IP and TCP fields are not kept. The Windows runner is Windows Server, so
`platformVersion` in the `navigator` block is not what a desktop Windows 10 or 11 reports; no
check reads it.

What Edge 156 sends differently from Google Chrome 156 (same Chromium, same HTTP/2 and
cipher suites):

- no `trust_anchors` extension (`0xca34`): 16 extensions, JA4 `t13d1516h2_8daaf6152771_806a8c22fdea`
  against Chrome's `t13d1517h2_8daaf6152771_cb7bf5808d99`; the extension order is shuffled per
  connection as in Chrome;
- the navigation `accept` has no `image/jxl`;
- the `sec-ch-ua` brand is `"Microsoft Edge"` and the User-Agent ends in `Edg/156.0.0.0`.

Platform-dependent fields: only the OS token of the User-Agent and `sec-ch-ua-platform`
(`"Windows"` measured on Windows, `"Linux"` on Linux); TLS, HTTP/2, header names, order and
every other value are the same on both. There is no macOS Edge capture and no `edge_156`
entry for macOS. Chromium sends `"macOS"` there (as the Chrome 156 macOS capture shows) and
the frozen `Macintosh; Intel Mac OS X 10_15_7` token, but that is Chrome's behaviour, not a
measurement of Edge. Also not measured: Edge on Android, Windows on ARM, a desktop Windows
client rather than a Server runner, and any Edge major other than 156.
