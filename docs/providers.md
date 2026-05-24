# Providers

Gi can use built-in providers through environment variables, command-line API keys,
or credentials saved by `/login`. Custom provider and model definitions can be
added with `~/.gi/agent/models.json`.

## Amazon Bedrock

Amazon Bedrock does not use a single Gi API key. Configure one of the AWS
credential mechanisms supported by the AWS SDK, then set the region if needed:

- `AWS_PROFILE`
- `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`
- `AWS_BEARER_TOKEN_BEDROCK`
- `AWS_REGION`

Run `gi --list-models bedrock` to confirm that Bedrock models are available.
