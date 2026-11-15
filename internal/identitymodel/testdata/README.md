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

Differences from upstream: values are visited in sorted order, not insertion
order; the total is the sum of the allowed entries even without a constraint;
the generator is deterministic. The distribution is the same, which
`TestSamplerMatchesUpstream` checks against 200k draws of the upstream code.

## Refreshing the data

The `identity-model-refresh` workflow does this monthly (2nd, the day after the
upstream rebuild) from the newest `header-generator` npm package that is at
least 24 hours old, and opens a PR; it never merges. By hand:

    npm pack header-generator@<version>   # data_files/input-network-definition.zip
    GBN_VERSION=<version> node oracle/apify_marginals.mjs <zip> > apify_reference.json
    go run ./cmd/identity-model-import -in <zip> -package header-generator@<version> \
        -committed <release date> -out internal/identitymodel/model.json
    go test . -run TestGenerateIdentity_Vectors -update-identity-vectors

The importer refuses a model whose newest Chrome is more than 2 majors behind
our newest built-in profile, or that lacks a Chrome major we ship. Review the
vector diff before committing: every changed row is an identity that moves for
a caller who re-derives from a seed.
