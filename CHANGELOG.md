# Changelog

## [1.28.1](https://github.com/anatolykoptev/go-stealth/compare/v1.28.0...v1.28.1) (2026-11-08)


### Fixed

* follow provider pagination only on the same origin ([#85](https://github.com/anatolykoptev/go-stealth/issues/85)) ([9ff01ac](https://github.com/anatolykoptev/go-stealth/commit/9ff01ac892ae2601e27b6d0f7cb196f799c95978))

## [1.28.0](https://github.com/anatolykoptev/go-stealth/compare/v1.27.0...v1.28.0) (2026-11-04)


### Added

* WithCookieJar shares one cookie jar across clients ([#80](https://github.com/anatolykoptev/go-stealth/issues/80)) ([07f3650](https://github.com/anatolykoptev/go-stealth/commit/07f36505690a39bd24538a2f32b41b51b3165e7b))

## [1.27.0](https://github.com/anatolykoptev/go-stealth/compare/v1.26.1...v1.27.0) (2026-11-04)


### Added

* match Chrome's TLS extension shuffle, fetch priority, zstd and add chrome_156 ([#74](https://github.com/anatolykoptev/go-stealth/issues/74)) ([c0549b9](https://github.com/anatolykoptev/go-stealth/commit/c0549b97c8d71327f48c45db18958299013efbcc))

## [1.26.1](https://github.com/anatolykoptev/go-stealth/compare/v1.26.0...v1.26.1) (2026-11-04)


### Fixed

* headers that match the client's own browser profile ([#72](https://github.com/anatolykoptev/go-stealth/issues/72)) ([b256cab](https://github.com/anatolykoptev/go-stealth/commit/b256caba2b2261268573468c1f232127e8f0fa91))

## [1.26.0](https://github.com/anatolykoptev/go-stealth/compare/v1.25.1...v1.26.0) (2026-11-04)


### Added

* coherent current browser profiles; fail on unknown profile names ([#70](https://github.com/anatolykoptev/go-stealth/issues/70)) ([4d10644](https://github.com/anatolykoptev/go-stealth/commit/4d106449165dec86d843b8aaf1eb764d2553ae67))

## [1.25.1](https://github.com/anatolykoptev/go-stealth/compare/v1.25.0...v1.25.1) (2026-11-03)


### Fixed

* **ssrf:** let a client dial its configured proxy on a private address ([#66](https://github.com/anatolykoptev/go-stealth/issues/66)) ([d65390c](https://github.com/anatolykoptev/go-stealth/commit/d65390c2b51bcbf670ced8e241088b8c1157e306))

## [1.25.0](https://github.com/anatolykoptev/go-stealth/compare/v1.24.0...v1.25.0) (2026-11-02)


### Added

* **client:** add CloseIdleConnections ([#64](https://github.com/anatolykoptev/go-stealth/issues/64)) ([c7eec72](https://github.com/anatolykoptev/go-stealth/commit/c7eec72c1c2cdff59e5c1c860e467332f563d30d))

## [1.24.0](https://github.com/anatolykoptev/go-stealth/compare/v1.23.0...v1.24.0) (2026-11-02)


### Added

* **deps:** move to Go 1.27 and close the backend ([#61](https://github.com/anatolykoptev/go-stealth/issues/61)) ([307927f](https://github.com/anatolykoptev/go-stealth/commit/307927f94f5b89a2c107feaf0b44eee63c70f0fa))

## [1.23.0](https://github.com/anatolykoptev/go-stealth/compare/v1.22.4...v1.23.0) (2026-10-10)


### Added

* **oxbrowser:** send X-Internal-Secret to ox-browser via go-kit svcauth (ox-browser[#173](https://github.com/anatolykoptev/go-stealth/issues/173)) ([#59](https://github.com/anatolykoptev/go-stealth/issues/59)) ([876152f](https://github.com/anatolykoptev/go-stealth/commit/876152f11ec9d51ac0f2665039dd53c5635d2d6d))

## [1.22.4](https://github.com/anatolykoptev/go-stealth/compare/v1.22.3...v1.22.4) (2026-10-08)


### Fixed

* **proxypool:** refresh Webshare credentials periodically and on 407 ([#55](https://github.com/anatolykoptev/go-stealth/issues/55)) ([#56](https://github.com/anatolykoptev/go-stealth/issues/56)) ([b46e7be](https://github.com/anatolykoptev/go-stealth/commit/b46e7beb099fd8f8b71071e141b838e5384632a8))

## [1.22.3](https://github.com/anatolykoptev/go-stealth/compare/v1.22.2...v1.22.3) (2026-08-04)


### Documentation

* **readme:** correct the profile count and drop links to private repos ([1032c7b](https://github.com/anatolykoptev/go-stealth/commit/1032c7bd488b11ef20b68f5bd566c2784556572a))

## [1.22.2](https://github.com/anatolykoptev/go-stealth/compare/v1.22.1...v1.22.2) (2026-07-27)


### Changed

* **pacing:** delegate jitter/pacer to go-kit/pacing ([#51](https://github.com/anatolykoptev/go-stealth/issues/51)) ([3bb7486](https://github.com/anatolykoptev/go-stealth/commit/3bb7486bf1be87aec9045ed9c1e0209da4b574fa))

## [1.22.1](https://github.com/anatolykoptev/go-stealth/compare/v1.22.0...v1.22.1) (2026-07-26)


### Fixed

* detect Cloudflare 403 managed challenges via cf-mitigated header ([#49](https://github.com/anatolykoptev/go-stealth/issues/49)) ([9471dd4](https://github.com/anatolykoptev/go-stealth/commit/9471dd4538b6e7a8c4bf510d76952d7a5dc36dd3))

## [1.22.0](https://github.com/anatolykoptev/go-stealth/compare/v1.21.2...v1.22.0) (2026-07-26)


### Added

* bump tls-client v1.14.0 -&gt; v1.15.1, add Firefox_148 and Brave_146 ([#46](https://github.com/anatolykoptev/go-stealth/issues/46)) ([c2037b5](https://github.com/anatolykoptev/go-stealth/commit/c2037b5855314fc566ddafbb6cd43077ed2d4a64))

## [1.21.2](https://github.com/anatolykoptev/go-stealth/compare/v1.21.1...v1.21.2) (2026-07-26)


### Fixed

* **backend:** strip h3 from Chrome 133 ClientHello to match cold-connection Chrome ([#44](https://github.com/anatolykoptev/go-stealth/issues/44)) ([40f0996](https://github.com/anatolykoptev/go-stealth/commit/40f099639dcf903a830a5d72a24bdd5e0f8ee100))

## [1.21.1](https://github.com/anatolykoptev/go-stealth/compare/v1.21.0...v1.21.1) (2026-07-26)


### Fixed

* match real Chrome's request-header set and accept header ([#42](https://github.com/anatolykoptev/go-stealth/issues/42)) ([75302aa](https://github.com/anatolykoptev/go-stealth/commit/75302aad1c7bcca86b4395ed33143825be29197c))

## [1.21.0](https://github.com/anatolykoptev/go-stealth/compare/v1.20.0...v1.21.0) (2026-07-26)


### Added

* give browser identity an owner (BrowserIdentity, WithIdentity, Identity, UserAgentForProfile) ([#37](https://github.com/anatolykoptev/go-stealth/issues/37)) ([8bc3e52](https://github.com/anatolykoptev/go-stealth/commit/8bc3e52b43d31b17044edbc11c1b8a573f54bf01))

## [1.20.0](https://github.com/anatolykoptev/go-stealth/compare/v1.19.1...v1.20.0) (2026-07-26)


### Added

* Chrome 144/146 profiles and three-brand sec-ch-ua ([#31](https://github.com/anatolykoptev/go-stealth/issues/31)) ([21dea00](https://github.com/anatolykoptev/go-stealth/commit/21dea0036a18b47e8070456f5cd9afc608bd7992))

## [1.19.1](https://github.com/anatolykoptev/go-stealth/compare/v1.19.0...v1.19.1) (2026-07-18)


### Fixed

* **oxbrowser:** check json.Marshal errors in Solve/FetchSmart/Analyze ([#26](https://github.com/anatolykoptev/go-stealth/issues/26)) ([c5c516d](https://github.com/anatolykoptev/go-stealth/commit/c5c516dab8d719d158ccf1b9296f433e5a8a9f44))
* **proxypool:** guard Webshare.Next() against empty pool panic ([#25](https://github.com/anatolykoptev/go-stealth/issues/25)) ([e67cca4](https://github.com/anatolykoptev/go-stealth/commit/e67cca4d3382fda02fc667f922447eae835ff6b8))
* **roundtripper:** preserve multi-value Set-Cookie headers correctly ([#28](https://github.com/anatolykoptev/go-stealth/issues/28)) ([9610d94](https://github.com/anatolykoptev/go-stealth/commit/9610d945e477fc19a274c6febe7d918386e1be9d))


### Changed

* consolidate extractDomain into shared internal/uri.ExtractHost ([#27](https://github.com/anatolykoptev/go-stealth/issues/27)) ([d722ab7](https://github.com/anatolykoptev/go-stealth/commit/d722ab7108f2b4002b8e0b16b42b7b1b9b720b1e))

## [1.19.0](https://github.com/anatolykoptev/go-stealth/compare/v1.18.1...v1.19.0) (2026-07-18)


### Added

* **backend:** std backend honors InsecureSkipVerify for opt-in parity ([#11](https://github.com/anatolykoptev/go-stealth/issues/11)) ([fe977f7](https://github.com/anatolykoptev/go-stealth/commit/fe977f7116222511d107030d82a8c36034e549af))


### Fixed

* **security:** secure-by-default TLS verification with opt-in skip-verify ([#7](https://github.com/anatolykoptev/go-stealth/issues/7)) ([368c09c](https://github.com/anatolykoptev/go-stealth/commit/368c09ce0c42510b9e44ede878d8a67227bb98e8))


### Documentation

* v2.0.0 breaking change — secure-by-default TLS verification ([#13](https://github.com/anatolykoptev/go-stealth/issues/13)) ([7deb852](https://github.com/anatolykoptev/go-stealth/commit/7deb852018eb309dc39da378a2c3e42a2baa6540))
