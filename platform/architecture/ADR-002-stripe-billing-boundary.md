# ADR-002: Stripe billing boundary for a future hosted product

- Status: proposed; not authorized for implementation
- Date: 2026-07-15
- Scope: a future multi-customer SaaS phase after ADR-001
- Deployment: prohibited in the local recovery lab

## Context

ADR-001 deliberately limits `platform/` to a local, single-workspace recovery
experiment. It uses a static development bearer token and a small object-backed
catalog to validate plan, transfer, commit, and recovery behavior. Those choices
are useful for the experiment but cannot establish customer identity, tenant
ownership, transactional quota, billing state, or safe multi-device access.

Adding a Stripe button to that laboratory would create a misleading product
surface: a payment could succeed without a durable account to own the purchase,
and the current catalog could not enforce paid limits atomically. Payment status
must never become a substitute for authorization or a reason to make recovery
data inaccessible.

The existing commercial hypothesis is:

| Plan | Price | Remote storage | Included download | Devices |
| --- | ---: | ---: | ---: | ---: |
| Local | $0 | None | None | Local machine only |
| Remote Vault Beta | $15/month | 50 GB | 5 GB/month | 3 |

This is a hypothesis to test, not a price or allowance implemented by this ADR.
Before launch, unit economics must include Stripe Payments and Billing fees,
Blob storage and transfer, Functions, Postgres, tax, refunds, support, and failed
or abandoned uploads. Vercel charges separately for Blob storage and outbound
transfer, so download behavior is a material margin risk.

## Decision

Do not install the Stripe SDK, provision Stripe or Neon resources, create
payment routes, add secrets, or display a functional checkout in the local
recovery laboratory.

A separate SaaS implementation phase may begin only after all of the following
gates are satisfied:

1. Real user identity replaces the shared development bearer token.
2. A transactional Neon Postgres control plane owns accounts, workspaces,
   billing projections, idempotency, reservations, quota, and object metadata.
3. Every request is authorized against an explicit account, workspace, vault,
   and device; no key or query defaults to `workspaces/default`.
4. Vercel Blob uploads use a staging path and have a proven
   verification/quarantine/repair lifecycle before promotion to an immutable
   final path.
5. Client-side encryption and recovery-key handling have an accepted design.
6. The billing and offboarding policy preserves recovery under payment failure.

Stripe will be a payment and subscription provider behind a narrow billing
port. It will not be an identity provider, authorization database, quota ledger,
or source of file metadata. Neon will hold the local projection used for access
decisions. Vercel Blob will hold encrypted archive bytes only.

## Checkout and customer management

The first paid version will use Stripe-hosted Checkout in `subscription` mode
and Stripe's hosted Customer Portal. Embedded Checkout and Elements are
deferred: hosted surfaces reduce implementation and PCI scope, work naturally
with a CLI-to-browser flow, and already support responsive layouts, local
payment methods, and Stripe-managed billing details.

The Checkout contract will accept an internal `planKey`, not a Stripe Price ID:

```text
remote-vault-beta-monthly -> STRIPE_PRICE_REMOTE_VAULT_BETA_MONTHLY
```

The mapping is a server-side allowlist. A client can never submit a customer ID,
arbitrary Price ID, amount, currency, account ID, or entitlement. The server
derives the authenticated billing account, reuses its mapped Stripe Customer,
and creates the Checkout Session with an idempotency key tied to a persisted
billing attempt.

Portal sessions are also created server-side and on demand. The server derives
the Stripe Customer from the authenticated billing account rather than trusting
a customer ID in the request. Portal URLs are short-lived and are never stored
as account state.

The success or return page is presentational. It may retrieve the Checkout
Session for display and poll the internal billing projection, but it never
grants access. Stripe explicitly requires webhook-backed fulfillment because a
customer is not guaranteed to reach the redirect page.

Initial portal capabilities will be intentionally narrow: update payment
method, view invoices, and cancel at period end. Plan switching, seat quantity,
coupons, prorations, and custom portal domains are deferred until there are
multiple validated plans.

## Webhook boundary

Stripe calls one dedicated integration endpoint. It does not use the normal
file.cheap bearer authentication. The handler must:

1. read the untouched raw request body;
2. verify the `Stripe-Signature` header with the endpoint signing secret;
3. reject a sandbox/live-mode mismatch;
4. normalize only an allowlisted event envelope;
5. insert it into a transactional inbox with `event.id` as a unique key;
6. acknowledge duplicates without repeating side effects; and
7. return success only after durable receipt of the event.

Stripe can retry an event, send semantic duplicates, and deliver events out of
order. Processing therefore does not apply event deltas in arrival order. For a
subscription or invoice event, the processor retrieves the current canonical
Stripe object where possible and projects that state idempotently into Neon.
Terminal deletion data is retained in normalized form. The inbox records event
ID, event type, Stripe object ID, Stripe creation time, live mode, processing
state, attempts, and a bounded error. Full webhook payloads are not retained by
default; if temporary encrypted retention becomes operationally necessary, it
must have a documented short TTL.

Outbound Stripe `POST` requests use persisted high-entropy idempotency keys.
Incoming webhook deduplication and outbound Stripe request idempotency solve
different problems and both are required.

The initial event allowlist is:

- `checkout.session.completed`;
- `customer.subscription.created`;
- `customer.subscription.updated`;
- `customer.subscription.deleted`;
- `customer.subscription.paused`;
- `customer.subscription.resumed`;
- `invoice.paid`;
- `invoice.payment_failed`; and
- `invoice.payment_action_required`.

`checkout.session.async_payment_succeeded` and
`checkout.session.async_payment_failed` become required only if delayed payment
methods are enabled. `customer.updated` is required only if file.cheap mirrors
a separate billing contact. Stripe Entitlements and its
`entitlements.active_entitlement_summary.updated` event are deferred; if later
adopted, entitlements are persisted internally and remain unsuitable as a
quantitative quota ledger.

Unknown, validly signed events are acknowledged and ignored. The Stripe event
destination subscribes only to event types used by the application.

## Transactional control-plane model

Neon becomes mandatory before any external customer or paid Blob usage. The
minimum logical schema is:

| Relation | Responsibility |
| --- | --- |
| `accounts` | Stable application principals mapped to the chosen identity provider. |
| `workspaces` | Tenant boundary and owner of vaults. |
| `memberships` | Account roles within a workspace. |
| `billing_accounts` | Billable owner, separate from a person so teams can be added later. |
| `devices` | Named CLI installations, public identity, last use, and revocation. |
| `device_authorizations` | Expiring, one-use browser approval challenges. |
| `device_sessions` | Hashed, rotating refresh-token state and revocation. |
| `stripe_customers` | Unique billing-account-to-Stripe-Customer mapping. |
| `stripe_subscriptions` | Current normalized subscription, product, price, status, and period. |
| `stripe_webhook_events` | Durable, idempotent webhook inbox and processing result. |
| `entitlements` | Internal feature projection and effective validity window. |
| `vaults` | Workspace-scoped encrypted vault identity. |
| `objects` | Opaque encrypted-object identity, verified byte size, hash, and storage state. |
| `stash_revisions` | Immutable stash-to-object references without plaintext names or paths. |
| `upload_reservations` | Expiring byte reservations and their idempotency keys. |
| `quota_balances` | Transactionally materialized committed and reserved bytes. |
| `usage_ledger` | Append-only storage accounting entries. |
| `tombstones` | Explicit deletion and retention lifecycle. |

Foreign keys, unique constraints, and transactions enforce ownership and
idempotency. A billing account has at most one current self-serve subscription.
Stripe customer and subscription IDs are unique. An upload idempotency key is
unique within its workspace.

Blob bytes, payment-card data, encryption keys, recovery secrets, plaintext
manifests, filenames, local paths, tags, and search indexes do not belong in
this schema.

## Upload reservation, quota, and ledger

Storage quota is enforced before issuing a signed upload URL:

1. authenticate the account and device and authorize workspace membership;
2. find or create an upload reservation by workspace and idempotency key;
3. in one database transaction, require
   `committed_bytes + reserved_bytes + requested_bytes <= storage_limit_bytes`;
4. increment reserved bytes and append `upload_reserved` to the ledger;
5. issue a narrow signed upload grant for a random staging pathname; and
6. bind workspace, vault, reservation, expected size, and expected ciphertext
   hash into the signed commit receipt.

Commit verifies that the staging object belongs to the reservation and meets
the accepted integrity protocol. A mismatch is quarantined, never promoted. A
successful commit transaction creates the immutable object/revision, moves
reserved bytes to committed bytes, and appends `object_committed`. Repeating the
same commit is a no-op with the same result.

Expired or abandoned reservations append `upload_released` and decrement the
reserved balance. A reconciler removes their staging objects. Deletion appends
`object_deleted` only after storage deletion is confirmed. Manual corrections
are explicit `manual_adjustment` entries; ledger rows are never rewritten. A
unique source reference prevents double accounting.

### Neon and Blob lifecycle

Neon and Blob do not share a transaction. The service must persist an explicit
state machine instead of trying to make a database commit and an object-store
mutation appear atomic:

```text
reserved → uploaded → verified → committed
                    ↘ quarantined
reserved/uploaded   → expired → releasing → released
```

Every transition carries one stable operation/idempotency key. The reservation
transaction creates `reserved`; the client then uploads once to a random,
non-overwritable pathname. After verification, a second Neon transaction locks
that reservation, records the immutable object and stash revision, moves bytes
from reserved to committed, writes the ledger entry, and marks the operation
`committed`. Retrying any step returns or advances the same operation.

Physical rename is not assumed. The random staging pathname is promoted
logically by the atomic `verified → committed` Neon transaction and retained as
the final object. If a future Blob primitive is used to copy or promote bytes,
that external step adds an explicit durable `promoting` state and is repaired by
the same reconciler; it cannot sit invisibly inside a database transaction.

An outbox/reconciler resumes `uploaded`, `verified`, `releasing`, and any future
`promoting` operations after process death. It quarantines integrity failures,
releases quota only once, removes abandoned staging bytes after the retention
window, and alerts on a committed reference whose object is missing. Neither a
timeout nor a webhook retry may decrement or increment quota twice.

The 50 GB plan storage limit is a hard limit over committed plus reserved
encrypted bytes. The 5 GB monthly download allowance is initially a product
hypothesis, not an invoice meter. Direct signed URLs do not by themselves give
file.cheap exact per-customer consumed-byte accounting, and an issued URL might
not be used or might be retried before expiry. `download_grant_issued` can be an
abuse and capacity signal, but it must not be represented as authoritative
download consumption or sent to Stripe for metered billing until measurement
is proven.

No automatic overage charges are introduced in the beta. Signed download URLs
use short expirations, and download-plan issuance is rate limited. Vercel Spend
Management supplies an independent infrastructure-cost guardrail.

## Recovery-first dunning policy

Subscription state controls new remote consumption, not ownership of existing
recovery data:

| Billing state | New uploads | List and download | Deletion/export |
| --- | --- | --- | --- |
| `active` or `trialing` | Allowed within quota | Allowed | Allowed |
| `past_due` | Allowed only during an explicit write grace period | Allowed | Allowed |
| `incomplete` | Blocked | Existing data only | Allowed |
| `paused`, `unpaid`, or `canceled` | Blocked | Allowed through the documented offboarding retention period | Allowed |

`trialing` is handled defensively if Stripe reports it; enabling a customer
trial remains a separate later product decision.

Cancellation scheduled at period end remains active until that period ends.
Webhook delay or a Stripe outage does not revoke a previously valid local
projection immediately. A reconciliation process repairs stale state later.

Payment failure never deletes, evicts, corrupts, or silently hides a remote
object. Offboarding requires clear notices, a reasonable retention window, an
export/recovery path, and an explicit tombstone lifecycle. Local CLI, MCP,
search, restore, and diff remain available without an account or subscription.

## Go CLI and device authorization

Stripe does not authenticate the Go CLI. A separate device authorization flow
links an installation to a file.cheap account:

1. `fcheap cloud login` requests an expiring device challenge.
2. The CLI displays a short user code and opens a verification URL.
3. The browser authenticates the user and shows the exact device and workspace
   being approved.
4. The CLI polls at the instructed interval and redeems the approved challenge
   exactly once.
5. The service returns a short-lived access token and a rotating refresh token;
   only a hash of the refresh credential is stored server-side.

Access tokens identify account, device, audience, and scopes. They do not carry
a Stripe secret, Stripe Customer ID, Price ID, or long-lived paid entitlement.
Each upload plan is authorized against current membership, billing policy, and
quota in Neon. Refresh-token replay revokes the affected device session. Users
can inspect and revoke devices independently of billing.

`fcheap cloud upgrade` may open the authenticated hosted Checkout flow, and
`fcheap cloud billing` may open an application page that creates a fresh Portal
session. The CLI never calls Stripe directly. Offline local commands never
depend on device auth or billing availability.

## Privacy and security boundary

The client encrypts archives before remote transfer. The hosted service sees
only opaque tenant/vault identifiers, encrypted byte counts, ciphertext hashes,
and the minimum operational state needed for recovery.

Stripe metadata contains only opaque internal references such as
`billing_account_id` and `billing_attempt_id`. It never contains filenames,
paths, stash IDs, archive hashes, tags, recovery-card contents, bearer tokens,
signed URLs, encryption material, or other secrets. Stripe also advises against
putting sensitive information in metadata.

Stripe secret keys and webhook signing secrets remain server-only, use separate
sandbox and live values, and are rotated. Logs redact authorization headers,
refresh tokens, signed transfer URLs, Stripe secrets, and raw webhook bodies.
The Blob and Neon regions, retention policy, subprocessors, tax handling, and
privacy disclosures must be decided and published before beta.

## Testing requirements

No paid beta is permitted until these automated tests exist:

- plan-key allowlisting and rejection of client-supplied price, amount,
  customer, or billing-account identifiers;
- authenticated Checkout retry returns one logical billing attempt;
- Portal customer lookup is derived server-side and resists cross-account IDOR;
- webhook raw-body signature verification, missing/invalid signature, sandbox
  versus live mismatch, duplicate delivery, concurrent duplicate delivery,
  semantic duplicate, unknown event, and out-of-order subscription events;
- redirect and success-page visits cannot grant an entitlement;
- a complete subscription policy matrix, including cancellation at period end,
  failed renewal, grace, and offboarding recovery;
- two concurrent reservations competing for the final available bytes allow
  only one, with no double reservation on retry;
- reservation expiry, staging cleanup, integrity mismatch quarantine,
  idempotent commit, confirmed deletion, and ledger reconciliation;
- workspace, vault, and object isolation between two accounts;
- device-code expiration, approval, one-use redemption, polling limits, refresh
  rotation, replay detection, and revocation; and
- payment failure blocks new paid consumption but never deletes data or blocks
  the promised recovery/export path.

Stripe sandbox integration tests use the Stripe CLI for signed webhook delivery
and Test Clocks for renewal, failure, cancellation, and grace-period scenarios.
Neon integration tests use an isolated database or branch and exercise real
transactions and unique constraints. End-to-end tests retain Bun as the project
runner and never use live Stripe keys or charge real payment methods.

## Rollout

1. **Document only:** keep this ADR proposed; add no providers or credentials to
   the current lab.
2. **Close platform gates:** prove encrypted bundle semantics, Blob staging and
   repair, real identity, tenant isolation, and the retention policy.
3. **Sandbox implementation:** add the billing port, Neon schema, and Stripe
   sandbox behind a disabled feature flag on a separately authorized branch.
4. **Internal dogfood:** use test clocks and synthetic archives; reconcile
   Stripe, Neon, and Blob state; validate cost assumptions and support runbooks.
5. **Founder beta:** create the Remote Vault Beta Product and monthly Price,
   cap enrollment, enable Vercel Spend Management, and monitor storage,
   transfer, reservation leakage, webhook lag, payment recovery, and restores.
6. **External beta:** proceed only after privacy/terms/tax review, backup and
   restore drills, incident response, key rotation, data export, deletion, and
   customer-support procedures are verified.

Annual pricing, multiple tiers, teams, seats, trials, promotion codes, metered
overages, Stripe Tax automation, and Stripe Entitlements remain later decisions.

## Consequences

- The local recovery experiment remains reproducible without external-service
  credentials; its development bearer and signing secret remain explicit.
- A payment cannot outrun identity, ownership, integrity, or quota controls.
- Stripe remains replaceable behind a billing port, while Neon is the fast,
  transactionally consistent source used for application authorization.
- Recovery remains available during billing failures and the documented
  offboarding retention window, which increases retained-storage cost but
  preserves the product's core promise.
- The first SaaS slice requires more control-plane work before any checkout can
  be honest; that work is an intentional launch gate rather than lab scope.
- Exact download-based billing is deferred because signed direct transfer does
  not yet provide authoritative per-customer consumption evidence.

## Official references

### Stripe

- [Checkout overview and hosted versus embedded UI](https://docs.stripe.com/payments/checkout)
- [Checkout fulfillment and webhook requirement](https://docs.stripe.com/checkout/fulfillment)
- [Checkout success-page limitations](https://docs.stripe.com/payments/checkout/custom-success-page)
- [Customer Portal integration](https://docs.stripe.com/customer-management/integrate-customer-portal)
- [Webhook signatures, retries, ordering, and duplicate events](https://docs.stripe.com/webhooks)
- [Subscription webhook events](https://docs.stripe.com/billing/subscriptions/webhooks)
- [Idempotent Stripe API requests](https://docs.stripe.com/api/idempotent_requests)
- [Stripe metadata security guidance](https://docs.stripe.com/metadata)
- [Stripe integration security](https://docs.stripe.com/security/guide)
- [Stripe Entitlements](https://docs.stripe.com/billing/entitlements)
- [Stripe Billing test clocks](https://docs.stripe.com/billing/testing/test-clocks)
- [Stripe CLI webhook testing](https://docs.stripe.com/stripe-cli/use-cli)
- [Stripe pricing for Mexico](https://stripe.com/mx/pricing)
- [Stripe Billing pricing for Mexico](https://stripe.com/mx/billing/pricing)

### Vercel

- [Stripe on the Vercel Marketplace](https://vercel.com/marketplace/stripe)
- [Neon on the Vercel Marketplace](https://vercel.com/marketplace/neon)
- [Vercel storage selection guidance](https://vercel.com/docs/storage)
- [Vercel Blob usage and pricing](https://vercel.com/docs/vercel-blob/usage-and-pricing)
- [Vercel Blob signed URLs](https://vercel.com/changelog/signed-urls-are-now-available-for-vercel-blob)
- [Vercel Blob client uploads](https://vercel.com/docs/vercel-blob/client-upload)
- [Vercel Spend Management](https://vercel.com/docs/spend-management)
