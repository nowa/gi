# Security Policy

## Supported Versions

Security fixes are accepted on the `main` branch. This repository does not
currently publish a separate long-term-support branch.

## Reporting a Vulnerability

Do not include secrets, provider tokens, or exploit details in a public issue.
Use GitHub private vulnerability reporting or contact the repository maintainers
privately when a private channel is available.

When reporting, include:

- affected package path, version, or commit
- a minimal reproduction or payload shape
- expected impact and whether credentials or network access are required

## Secrets and Live Providers

Default tests must not require live credentials or network calls. Keep any
credentialed provider checks outside the default test suite, and never commit
API keys, OAuth tokens, session dumps, or provider responses containing secrets.
