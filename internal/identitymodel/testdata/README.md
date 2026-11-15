# Identity model: data and sampling specification

`../model.json` is the model; `apify_reference.json` is the reference output of
the upstream sampler (`oracle/apify_marginals.mjs`); the vectors one level up
(`../../../testdata/identity_model_vectors.json`) pin seed to identity.

## File format (`identity-model/1`)

`networks.<name>.nodes` is a list in sampling order. A node has `name`,
`parents` (all earlier), `values` (sorted strings) and `cpt`. The CPT is a tree
with one level per parent, in `parents` order. A level is `{"d": {parentValue:
subtree}, "s": subtree}`; the last level is a leaf `[[valueIndex, p], ...]`
sorted by index. To find a node's leaf walk the levels: `d[value of the
parent]` if present, otherwise `s`. No `s` and no match means no leaf (dead).

## Sampling (must match across languages)

* Generator: SplitMix64, `state += 0x9E3779B97F4A7C15`, then the usual two
  xor-shift-multiply rounds (`0xBF58476D1CE4E5B9`, `0x94D049BB133111EB`). First
  outputs for seed 0: `e220a8397b1dcdaf 6e789e6aa1b965f4 06c45d188009454f`.
* `float64() = (next() >> 11) * 2^-53`.
* Node `i`: take the leaf entries in stored order, drop banned values and
  those the constraint refuses, `total` = sum of the remaining probabilities.
  Draw `anchor = float64() * total`; walk the entries accumulating `p`; the
  first entry where the running sum is `> anchor` is chosen (the last entry if
  none is). One `float64()` per attempt.
* If node `i+1` ends with no candidate, ban the value chosen at `i` and draw
  node `i` again from the remainder (one more `float64()`); if node `i` has
  no candidate left it fails to its parent. The constraint may look at the
  values already chosen.
* After the network, the identity layer draws one more `float64()` and picks
  `locales[floor(u * len)]`.

## Building the pool (the identity layer)

The network alone gives a (device, OS, browser/version) cell. The identity
layer decides which cells may be drawn and what each one means.

1. **Candidate entries.** An ordered list of entries `(browser, os, mobile,
   user_agent, tls_profile)`: the built-in profiles in table order, then the
   legacy profiles. Keep an entry only if its TLS profile is allowed (default:
   the TLS profiles of the built-in list, so legacy ones need to be named), its
   OS is in the OS constraint when one is given (compared in lower case), and
   it is not mobile unless mobile is enabled.
2. **Model value of an entry.** `chrome`: `chrome/<major>.0.0.0|2`; `edge`:
   `edge/<major>.0.0.0|2` with the major of the `Edg/` token; `firefox`:
   `firefox/<major>.<minor>|2` from `Firefox/`; `safari`: `safari/<v>|2` with `v`
   the `Version/` token. Any other browser (Brave sends a Chrome UA) has no
   value and is skipped. `|2` is the HTTP version the TLS backends speak.
3. **Cells.** Cell = `(os, device, value)` with `device` `"mobile"` when the
   entry is mobile, else `"desktop"`. When two entries share a cell, the first
   in list order wins and the rest are ignored.
4. **`allow(node, partial, value)`** for the sampler: `*DEVICE` is allowed when
   some cell has that device; `*OPERATING_SYSTEM` when some cell has
   `(value, partial[*DEVICE])`; `*BROWSER_HTTP` when the cell
   `(partial[*OPERATING_SYSTEM], partial[*DEVICE], value)` exists; every other
   node is always allowed.
5. **Weight floor.** `floor` is `0.01` when unset, none when negative.
   Repeat: compute the exact distribution of the network under `allow`
   (outcomes are removed when they dead-end, survivors renormalised level by
   level); the weight of a cell is the sum of the probabilities of the
   outcomes with that `(OS, DEVICE, BROWSER_HTTP)`. No outcome at all means no
   identity. Take the lightest cell, breaking ties by `os`, then `device`, then
   `value`, each in byte order. If its weight is at least `floor`, stop;
   otherwise delete that cell from the pool and repeat. One cell per round,
   because deleting one raises the others.
6. **Draw.** Sample the network under `allow`, look up the cell, take its
   entry, then draw the locale (above). A sample whose cell is missing is an
   error (it cannot happen after step 4).

Locales are canonicalised before use: language lower case, a four-letter script
in title case, a two-letter region in upper case, other subtags in lower case.
Accept-language is the tag, then `,<language>;q=0.9` when the tag has a
subtag.

The pool version the Go implementation prints is a hash of its own cells and
floor; it labels one implementation's pool and is not part of this contract.

Differences from upstream: values are visited in sorted order, not insertion
order; the total is the sum of the allowed entries even without a constraint;
the generator is deterministic. The distribution is the same, which
`TestSamplerMatchesUpstream` checks against 200k draws of the upstream code.

## Refreshing the data

The `identity-model-refresh` workflow does this monthly (2nd, the day after the
upstream rebuild) from the newest `header-generator` npm package that is at
least 24 hours old, and opens a PR; it never merges. By hand:

    npm -v                                # must be 11.10 or newer: older npm ignores
                                          # --min-release-age (Node 26 bundles 11.20)
    npm pack header-generator@<version>   # data_files/input-network-definition.zip
    cd oracle && npm install --package-lock-only --ignore-scripts --min-release-age 1 \
        --save-exact generative-bayesian-network@<version> && npm ci --ignore-scripts
    go run ./cmd/identity-model-import -pool-out internal/identitymodel/testdata/oracle/pool.json
    node oracle/apify_marginals.mjs <zip> > apify_reference.json
    go run ./cmd/identity-model-import -in <zip> -package header-generator@<version> \
        -committed <release date> -out internal/identitymodel/model.json
    go test . -run TestGenerateIdentity_Vectors -update-identity-vectors

The importer refuses a model whose newest Chrome is more than 2 majors behind
our newest built-in profile, or that lacks a Chrome major we ship. Review the
vector diff before committing: every changed row is an identity that moves for
a caller who re-derives from a seed.
