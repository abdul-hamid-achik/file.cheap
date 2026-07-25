# Email delivery

file.cheap can receive mail at `hello@file.cheap` and forward a bounded,
rendered copy to one private operator address. This optional hosted path is
separate from the local-first CLI and MCP server; saving, searching, and
restoring stashes never requires email or a network connection.

`POST /api/webhooks/resend` is a private provider callback exception outside
the public `/api/v1` API. Signed `email.received` events are filtered to the
exact `hello@file.cheap` recipient before file.cheap records replay state or
fetches message metadata. The route uses a fixed sender, preserves a validated
original Reply-To, and blocks loops.

Mail content is untrusted input. file.cheap does not log or store message
content, private destinations, or provider IDs. It retains only 90-day,
pseudonymized, joinable SHA-256 replay digests instead of plaintext identifiers. Attachments are deliberately omitted; use file.cheap
stashes for files instead. Delivery has replay and provider idempotency
protection, but distributed systems cannot promise exactly-once delivery.
Resend may deliver a webhook up to eight times; the bounded replay ledger covers
that provider retry window.

The Resend receiving credential is team-wide full access, not domain-scoped.
Current code reads it only in the Production webhook path alongside the
endpoint-specific secret and private destination. Vercel Production variables
are project/server-runtime scoped, so other same-project server code could
access `process.env`; true route isolation needs a separate deployment or
secret broker. Operational setup, DNS, verification, and recovery procedures
are maintained in the deployment runbook.
