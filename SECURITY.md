# Security policy

## Report a vulnerability

Please report suspected vulnerabilities privately through
[GitHub Security Advisories](https://github.com/abdul-hamid-achik/file.cheap/security/advisories/new).
Do not open a public issue with exploit details, credentials, private file
contents, or other sensitive evidence.

Include the affected version, operating system, a minimal reproduction, the
expected impact, and any suggested mitigation. You should receive an initial
acknowledgement through GitHub within seven days.

## Scope

Security reports may cover the `fcheap` CLI, MCP server, archive extraction,
path validation, secret handling, local database or search indexes, release
artifacts, and the documentation site. Reports about third-party tools or
services should be sent to their maintainers unless file.cheap's integration is
the source of the vulnerability.

## Inbound email

The hosted platform forwards only signed Resend events for
`hello@file.cheap`. It never exposes a generic send endpoint, renders inbound
HTML, downloads attachments, converts messages into stashes, or passes mail to
an agent. Other catch-all recipients are acknowledged without forwarding.

The private destination and provider credentials are Sensitive Production
configuration. Mail metadata and content must not appear in logs, database
rows, error responses, artifacts, or repository fixtures. Resend remains a
third-party storage and delivery boundary for any message sent to the domain.

## Disclosure

Please allow time to investigate and release a fix before publishing details.
When appropriate, a GitHub Security Advisory will document affected versions,
the remediation, and credit for the reporter.
