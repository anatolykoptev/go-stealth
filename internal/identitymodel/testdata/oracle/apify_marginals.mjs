// Draws samples from the upstream fingerprint-suite implementation
// (generative-bayesian-network, Apache-2.0) and prints joint counts, the
// reference the Go sampler is tested against. The Go test does not run this;
// the output is committed as apify_reference.json.
//
//   npm install generative-bayesian-network@2.1.90
//   node apify_marginals.mjs input-network-definition.zip > ../apify_reference.json
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const { BayesianNetwork } = require('generative-bayesian-network');
const VERSION = process.env.GBN_VERSION ?? '2.1.90'; // the npm version this was run with

const N = 200000;
const net = new BayesianNetwork({ path: process.argv[2] });
const chromeSet = ['chrome/156.0.0.0|2', 'chrome/152.0.0.0|2', 'chrome/150.0.0.0|2', 'chrome/146.0.0.0|2', 'edge/152.0.0.0|2'];
const scenarios = {
  desktop_chromium: {
    '*DEVICE': ['desktop'],
    '*OPERATING_SYSTEM': ['windows', 'macos', 'linux'],
    '*BROWSER_HTTP': chromeSet,
  },
  dead_end_backtrack: {
    '*DEVICE': ['desktop', 'mobile'],
    '*OPERATING_SYSTEM': ['linux'],
    '*BROWSER_HTTP': chromeSet.slice(0, 2),
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
