# Security Policy

## Supported version

Security fixes are applied to the latest public-preview release and `main`.
Older laboratory tags are retained as historical evidence and are not supported
for new deployments.

## Reporting a vulnerability

Use GitHub's **Report a vulnerability** private reporting flow for this
repository. Do not open a public issue containing an Agent token, model key,
personal memory, database, exported archive or exploitable reproduction.

Include the affected version, operating system, expected boundary, observed
behavior and the smallest safe reproduction. Replace all real secrets and
personal Evidence with synthetic values.

## Important deployment boundaries

- HTTP and MCP listeners must remain on loopback unless an operator has added
  a separately reviewed authentication and network boundary.
- Owner sessions and Agent tokens are different authorities. A browser reaching
  the local port does not receive Owner access.
- Tokens and model keys must be held in OS secret storage, an environment
  secret or a supported secret reference; never commit them or capture them as
  Evidence.
- Public-preview binaries are not currently Developer ID/notarized or
  Authenticode-signed. Verify SHA-256 values from the release before running.
- Memory Harness does not treat model output or retrieved content as trusted
  instructions. Protected memory, conflicts and Agent assets still require
  Owner review.

## Scope

The following are security issues: authorization bypass, cross-project data
exposure, private collaboration draft exposure, token leakage, non-loopback
binding without explicit configuration, Evidence mutation, unsafe archive
restore, or activation of protected content without Owner review.

Model quality disagreements, expected public-preview signing warnings and local
feature-embedding relevance differences are not vulnerabilities by themselves,
but are welcome as ordinary issues when no private data is included.
