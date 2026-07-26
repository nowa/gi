# Pi v0.82.1 Parity Baseline

Gi's closed catch-up baseline is the immutable Pi release:

- tag: `v0.82.1`
- commit: `b4f293684bba718d59cc1157679bcf6157b3a7f5`
- repository: `https://github.com/earendil-works/pi.git`

The machine-readable scope and intentional exclusions are in `baseline.json`.
The current debt snapshot is in `v0.82.1-open-gaps.json`.

## Closed baseline and drift detection

The source, module-boundary, and test-case verifiers predate Pi v0.82.0. The
original closure opened with 1,545 audit leads; the v0.82.1 catch-up introduced
nine new source/test mapping leads. The current snapshot is closed:

| Module | Open items |
| --- | ---: |
| LLM provider | 0 |
| Agent core and harness | 0 |
| TUI | 0 |
| Coding agent | 0 |

The 1,545 starting items were audit leads, not 1,545 proven behavioral bugs.
Each is now classified as an audited Gi implementation, a named test mapping,
a documented Go-native equivalent, or an explicit product-scope decision. In
particular, Pi's optional TypeScript example extension and Node `signal-exit`
listener behavior are recorded as scoped exclusions; Gi's protocol extension
boundary and Go `os/signal`/`sync.Once` lifecycle are the corresponding
product/runtime decisions.

The closed snapshot proves the audited source/member/module/test-mapping gate
for this exact commit. It is not a claim that unrelated Pi packages, live
credentialed providers, or byte-for-byte TypeScript APIs are implemented.

CI compares the live verifier output with the committed snapshot:

- a new unrecorded gap fails;
- a resolved gap that remains in the snapshot fails;
- unchanged known debt passes;
- release mode fails until both the live and recorded gap sets are empty.

This keeps normal development green without letting parity debt grow or silently
pretending that existing debt is complete.

## Local verification

Use a clean checkout of the exact Pi release. Do not generate the snapshot from
a moving or dirty `main` checkout.

```sh
git clone --branch v0.82.1 --depth 1 \
  https://github.com/earendil-works/pi.git /private/tmp/pi-v0.82.1

node docs/pi-parity/verify-pi-baseline.mjs \
  --pi-root /private/tmp/pi-v0.82.1

node --test docs/pi-parity/parity-baseline-lib.test.mjs

node docs/pi-parity/verify-parity-baseline.mjs \
  --pi-root /private/tmp/pi-v0.82.1
```

After a Pi or Gi parity change, regenerate the deterministic snapshot and
review any drift:

```sh
node docs/pi-parity/snapshot-parity-gaps.mjs \
  --pi-root /private/tmp/pi-v0.82.1 \
  --out docs/pi-parity/v0.82.1-open-gaps.json
```

The final v0.82.1 release gate is:

```sh
node docs/pi-parity/verify-parity-baseline.mjs \
  --pi-root /private/tmp/pi-v0.82.1 \
  --require-closed
```

The verifiers inspect Pi source and tests as data. They do not install or
execute Pi dependencies.
