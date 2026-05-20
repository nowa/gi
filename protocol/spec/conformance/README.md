# Gi Extension Protocol Conformance

This directory defines how an implementation proves compatibility with
`gi-ext-rpc@1` and `gi-viewtree@1`.

## Runner Modes

- `schema`: validate manifests, RPC envelopes, host actions, ViewTrees, and
  reports against `../schemas/*.schema.json`.
- `transcript`: replay `../examples/*.jsonl` and compare message ordering,
  result payloads, errors, capability denials, and diagnostics.
- `render`: mount ViewTree fixtures at deterministic terminal sizes and compare
  rendered snapshots.

## Required Suites

1. Manifest parsing, source normalization, resource filtering, and precedence.
2. Capability grant, deny, runtime approval, and audit behavior.
3. Extension handshake, version negotiation, diagnostics, and shutdown.
4. Lifecycle event ordering with stable `eventSeq`.
5. Tool, command, shortcut, flag, provider, and resource registration.
6. Tool execution, streaming updates, cancellation, and errors.
7. Host actions for tools, sessions, agents, model selection, TUI, policy,
   process execution, and filesystem access.
8. ViewTree node validation, render snapshots, patching, unknown-node fallback,
   and stale mount behavior.
9. Focus, key, text input, submit, cancel, resize, theme, visibility, and tick
   events.
10. Editor slot text APIs, autocomplete context, and submit ownership.
11. Message and tool renderer fallback behavior.
12. Extension crash, timeout, restart policy, and duplicate shutdown handling.
13. Pi ecosystem fixture ports for plan mode, todo, footer, custom editor,
   approval gate, subagents, MCP adapter, and provider extension.

## Evidence

A conforming implementation emits a report matching
`../schemas/conformance-report.schema.json`. Reports MUST include the
implementation identity, claimed profiles, feature matrix, fixture set version,
and per-suite pass/fail counts.

Compatibility claims are profile-scoped. Passing `gi-extension-host@1` does not
imply `gi-viewtree-renderer@1` unless that profile is also present in the
report.
