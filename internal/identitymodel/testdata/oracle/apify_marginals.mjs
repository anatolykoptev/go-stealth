// Draws samples from the upstream fingerprint-suite implementation
// (generative-bayesian-network, Apache-2.0) and prints joint counts, the
// reference the Go sampler is tested against. The Go test does not run this;
// the output is committed as apify_reference.json.
//
//   npm ci --ignore-scripts        # package-lock.json pins the sampler and its dependencies
//   node apify_marginals.mjs input-network-definition.zip > ../apify_reference.json
//
// The constraints come from pool.json, written by
// `go run ./cmd/identity-model-import -pool-out <this directory>/pool.json`
// from the library's built-in profiles; a test fails when the file is stale.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
const require = createRequire(import.meta.url);
const { BayesianNetwork } = require('generative-bayesian-network');
const here = new URL('.', import.meta.url);
const VERSION = JSON.parse(readFileSync(new URL('package.json', here))).dependencies['generative-bayesian-network'];
const pool = JSON.parse(readFileSync(new URL('pool.json', here)));

const N = 200000;
const net = new BayesianNetwork({ path: process.argv[2] });
const scenarios = {
  desktop_chromium: {
    '*DEVICE': ['desktop'],
    '*OPERATING_SYSTEM': pool.desktop_os,
    '*BROWSER_HTTP': pool.browser_http,
  },
  // Mobile has no linux entry in the model, so every mobile draw dead-ends and
  // the sampler must back off to desktop.
  dead_end_backtrack: {
    '*DEVICE': ['desktop', 'mobile'],
    '*OPERATING_SYSTEM': ['linux'],
    '*BROWSER_HTTP': pool.browser_http.slice(0, 2),
  },
  unconstrained: {},
};
const out = { generator: `generative-bayesian-network@${VERSION}`, samples: N, scenarios: {} };
for (const [name, constraints] of Object.entries(scenarios)) {
  const counts = {};
  for (let i = 0; i < N; i++) {
    const s = net.generateConsistentSampleWhenPossible(JSON.parse(JSON.stringify(constraints)));
    const k = [s['*DEVICE'], s['*OPERATING_SYSTEM'], s['*BROWSER_HTTP']].join('~');
    counts[k] = (counts[k] ?? 0) + 1;
  }
  out.scenarios[name] = { constraints, counts };
}
console.log(JSON.stringify(out, null, 1));
