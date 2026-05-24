# Models

Gi resolves models from the built-in provider catalog and optional custom model
definitions in `~/.gi/agent/models.json`.

Use `gi --list-models` to inspect available models, or filter the list:

```sh
gi --list-models openai
gi --list-models sonnet
```

In the TUI, use `/model` to select a model and `/scoped-models` to choose the
models available through Ctrl+P cycling.

Custom provider credentials and model definitions are configured through
`~/.gi/agent/models.json`. Provider login and credential setup details are in
`docs/providers.md`.
