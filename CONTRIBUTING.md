# Contributing

Thanks for contributing to Gi. This repository is compatibility-oriented, so
changes should preserve the documented Pi behavior unless the difference is
intentional and documented.

## Development Flow

1. Keep changes scoped to one package or compatibility area.
2. Add or update deterministic tests beside the implementation.
3. Run the relevant package tests, then `go test -timeout 30s ./...` before
   submitting broad changes.
4. Update `PI_COMPATIBILITY.md` or the parity reports when behavior coverage
   changes.

## Local Commands

```sh
go test -timeout 30s ./...
GOCACHE=/private/tmp/gi-gocache go test -timeout 30s ./...
gofmt -w path/to/file.go
```

## Credentials

Default tests must not require live provider credentials or network access. Do
not commit API keys, OAuth tokens, session dumps, or provider responses that may
contain secrets.

## Agent Instructions

See `AGENTS.md` for repository-specific coding-agent guidance, package layout,
test conventions, and pull request expectations.
