# Security Policy

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Report vulnerabilities privately through GitHub:
**Security → Report a vulnerability** on
[github.com/memrohq/memro](https://github.com/memrohq/memro/security/advisories/new).

Include what you found, how to reproduce it and the impact you expect.
We aim to acknowledge reports within 3 business days and will keep you
updated until a fix is released. We are happy to credit reporters in the
release notes.

## Supported versions

Security fixes go into the latest release. Self-hosted users should upgrade
to the newest version; database migrations run automatically on startup.

## Scope

In scope: the server (`server/`), SDKs (`sdk/`), the MCP server (`mcp/`) and
the deployment configs in this repository. Issues in third-party
dependencies should be reported upstream, but tell us if Memro is affected.
