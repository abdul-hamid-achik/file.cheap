# `fcheap auth`

Pair a local CLI with the private file.cheap artifact console. The flow is
designed for terminals, agents, and input-constrained devices: the CLI shows a
short pairing code, while the owner proves control of an allowlisted email and
explicitly approves the request in a browser.

## Login

```sh
fcheap auth login --service-url https://file.cheap
```

Or configure the origin for the current process:

```sh
FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap fcheap auth login
```

The command prints a verification URL and a code such as `F7KQ-8P2M`. Visit
the URL on any browser, enter the code and owner email, then enter the
six-digit code sent by email. Opening the email does not approve the device.

After explicit approval, the CLI redeems a one-time device grant. It stores a
short-lived access token and a rotatable refresh token at
`$XDG_CONFIG_HOME/fcheap/credentials.json` (or the
equivalent `~/.config/fcheap` path) with mode `0600`. The raw credential is
never printed, added to an `ArtifactRefV1`, or saved in the local vault.

## Status

```sh
fcheap auth status
```

This checks the stored access token against the configured service and prints
the authenticated owner email. If the access token expired, the command rotates
the refresh token, persists the replacement atomically, and retries. It does not
reveal either token.

## Refresh

```sh
fcheap auth refresh
```

Force a refresh-token rotation and issue a new 15-minute access token. The CLI
persists the proposed replacement and rotation ID before contacting the service,
so a retry after an ambiguous network failure repeats the same rotation instead
of invalidating the device. Reusing a replaced refresh token with different
rotation data revokes the entire device family.

## Logout

```sh
fcheap auth logout
```

Logout revokes the remote device family and all of its access tokens before
deleting the local credential.
If the remote service is unavailable, the local credential is retained so the
command can retry revocation instead of silently leaving an active token.

## Security boundaries

- Pairing codes expire after ten minutes and cannot authorize a device alone.
- Device polling is bounded and rate-limited.
- Browser sessions use an HttpOnly cookie and never receive device refresh
  credentials.
- Device access tokens expire after 15 minutes. Refresh tokens rotate on every
  use, expire after 30 idle days, and cannot outlive their 90-day family.
- A lost refresh token is replaced through a new email pairing. The service
  stores only HMAC digests and cannot recover or email an existing secret.
- `ArtifactRefV1` remains credential-free and never grants cloud access.
