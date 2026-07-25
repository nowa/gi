# Pi v0.82.0 Parity Baseline

Gi's current catch-up target is the immutable Pi release:

- tag: `v0.82.0`
- commit: `083e61621276bff9f6faefab87ce07fcd98734e2`
- repository: `https://github.com/earendil-works/pi.git`

The machine-readable scope and intentional exclusions are in `baseline.json`.
The current debt snapshot is in `v0.82.0-open-gaps.json`.

## Why the debt snapshot exists

The source, module-boundary, and test-case verifiers predate Pi v0.82.0. The
baseline opened with 1,545 audit items; the current snapshot contains 420:

| Module | Open items |
| --- | ---: |
| LLM provider | 0 |
| Agent core and harness | 0 |
| TUI | 0 |
| Coding agent | 420 |

These are audit items, not 1,545 proven behavioral bugs. A source file, symbol,
or test remains open until Gi either implements and verifies the behavior,
records the Go-native equivalent, or makes an explicit product-scope decision.
Refreshing the complete v0.82.0 test inventory closes only
`test-undocumented-file` bookkeeping; it does not by itself claim behavioral
coverage. Unmatched files/cases and source gaps remain in the snapshot.

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
git clone --branch v0.82.0 --depth 1 \
  https://github.com/earendil-works/pi.git /private/tmp/pi-v0.82.0

node docs/pi-parity/verify-pi-baseline.mjs \
  --pi-root /private/tmp/pi-v0.82.0

node --test docs/pi-parity/parity-baseline-lib.test.mjs

node docs/pi-parity/verify-parity-baseline.mjs \
  --pi-root /private/tmp/pi-v0.82.0
```

After a parity change resolves mapped debt, regenerate the deterministic
snapshot and review that it only shrinks:

```sh
node docs/pi-parity/snapshot-parity-gaps.mjs \
  --pi-root /private/tmp/pi-v0.82.0 \
  --out docs/pi-parity/v0.82.0-open-gaps.json
```

The final v0.82.0 release gate is:

```sh
node docs/pi-parity/verify-parity-baseline.mjs \
  --pi-root /private/tmp/pi-v0.82.0 \
  --require-closed
```

The verifiers inspect Pi source and tests as data. They do not install or
execute Pi dependencies.
