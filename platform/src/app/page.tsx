import { RecoveryLab } from "@/features/sync/recovery-lab";
import { getConfig } from "@/shared/config/env";

export const dynamic = "force-dynamic";

const protocolSteps = [
  {
    number: "01",
    title: "Plan the transfer",
    copy: "The control plane validates development authorization, size, hash, and current catalog state before any archive moves.",
  },
  {
    number: "02",
    title: "Move immutable bytes",
    copy: "Archive bytes take a short-lived direct path. Content-derived keys make retries safe and conflicts visible.",
  },
  {
    number: "03",
    title: "Commit the reference",
    copy: "Only an object that passes the active adapter checks can enter the catalog. The local stash remains untouched and authoritative.",
  },
  {
    number: "04",
    title: "Recover and prove",
    copy: "Download the archive, hash it again, and compare the result with the original recovery contract.",
  },
] as const;

const billingFlow = [
  ["Account", "A real user and workspace boundary"],
  ["Checkout", "Stripe-hosted purchase and customer portal"],
  ["Entitlement", "Verified billing state projected in Neon"],
  ["Device", "CLI authorization reads current access"],
] as const;

export default function HomePage() {
  const config = getConfig();

  return (
    <main>
      <nav className="nav shell" aria-label="Primary navigation">
        <a className="wordmark" href="https://file.cheap" aria-label="file.cheap home">
          <span className="wordmarkMark" aria-hidden="true">f·</span>
          <span>file.cheap</span>
        </a>
        <div className="navLinks">
          <a href="#recovery-lab">Recovery lab</a>
          <a href="#protocol">Protocol</a>
          <a href="#business-model">Business model</a>
        </div>
        <div className="navMeta">
          <span className="statusDot" aria-hidden="true" />
          research build · local only
        </div>
      </nav>

      <section className="hero shell" aria-labelledby="hero-title">
        <div className="heroCopy">
          <div className="eyebrow">Optional remote vault / protocol v1</div>
          <h1 id="hero-title">
            Keep the local copy.<br />
            <em>Prove the remote one.</em>
          </h1>
          <p className="heroLead">
            A recovery-first experiment for moving immutable file.cheap stashes through
            a provider-neutral protocol, then downloading the archive and proving its
            exact SHA-256. Nothing here deletes, evicts, or weakens the local vault.
          </p>
          <div className="heroActions">
            <a className="button primary" href="#recovery-lab">Run a recovery drill</a>
            <a className="button secondary" href="#protocol">Inspect the boundaries</a>
          </div>
          <dl className="heroFacts" aria-label="Prototype facts">
            <div><dt>{config.storageDriver}</dt><dd>active adapter</dd></div>
            <div><dt>SHA-256</dt><dd>recovery contract</dd></div>
            <div><dt>zero</dt><dd>local files removed</dd></div>
          </dl>
        </div>

        <aside className="receiptScene" aria-label="Example recovery receipt">
          <div className="receiptGlow" aria-hidden="true" />
          <div className="receiptCard">
            <div className="receiptTopline">
              <span>Illustrative drill report</span>
              <span className="verifiedBadge"><i aria-hidden="true">✓</i> example passed</span>
            </div>
            <div className="receiptTitle">
              <span className="receiptGlyph" aria-hidden="true">◫</span>
              <div>
                <strong>agent-session.fcheap</strong>
                <span>immutable archive · 18.4 MB</span>
              </div>
            </div>
            <div className="receiptHash">
              <span>SHA-256</span>
              <code>9f27 2b11 8a6e ··· 0e44 b31c</code>
            </div>
            <ol className="receiptTimeline">
              <li className="complete"><span>Local archive selected</span><b>01</b></li>
              <li className="complete"><span>Remote object committed</span><b>02</b></li>
              <li className="complete"><span>Archive downloaded</span><b>03</b></li>
              <li className="complete"><span>Bytes matched locally</span><b>04</b></li>
            </ol>
            <div className="receiptFooter">
              <span>proof, not a promise</span>
              <span>protocol v1</span>
            </div>
          </div>
            <div className="receiptCaption">
              <span aria-hidden="true">↳</span>
              A recovery card is a portable contract. The local drill report is observational
              evidence, not a tamper-evident receipt.
          </div>
        </aside>
      </section>

      <section className="trustRail" aria-label="Design guarantees">
        <div className="shell">
          <span>Local-first by default</span>
          <span>Provider-neutral grants</span>
          <span>Content-addressed objects</span>
          <span>Deep verification required</span>
        </div>
      </section>

      <section className="shell labSection" id="recovery-lab" aria-labelledby="lab-title">
        <div className="sectionHeading splitHeading">
          <div>
            <div className="eyebrow">The complete recovery loop</div>
            <h2 id="lab-title">Trust the proof.<br />Not the progress bar.</h2>
          </div>
          <p>
            This browser lab exercises the same authenticated HTTP grants a future Go
            client can consume. Use a small test archive, complete all five steps, and
            keep the generated recovery card beside the stash manifest.
          </p>
        </div>
        <RecoveryLab />
      </section>

      <section className="shell protocolSection" id="protocol" aria-labelledby="protocol-title">
        <div className="sectionHeading">
          <div className="eyebrow">A deliberately narrow protocol</div>
          <h2 id="protocol-title">Four transitions.<br />One invariant.</h2>
          <p className="sectionLead">
            Remote bytes may extend the local workflow; they never become permission to
            destroy its source of truth.
          </p>
        </div>
        <div className="protocolGrid">
          {protocolSteps.map((step) => (
            <article key={step.number}>
              <span className="stepNumber">{step.number}</span>
              <div className="protocolIcon" aria-hidden="true"><i /></div>
              <h3>{step.title}</h3>
              <p>{step.copy}</p>
            </article>
          ))}
        </div>
        <div className="boundaryGrid">
          <article className="boundaryCard localBoundary">
            <div className="boundaryLabel"><span aria-hidden="true">●</span> now · local lab</div>
            <h3>Blob-shaped without Blob lock-in</h3>
            <p>
              The protocol talks in plans, receipts, and transfer grants. The active
              <strong> {config.storageDriver}</strong> adapter can be replaced without changing
              the future CLI contract.
            </p>
            <ul>
              <li>Bearer-protected control plane</li>
              <li>Immutable, hash-derived object keys</li>
              <li>Restart-safe local catalog</li>
            </ul>
          </article>
          <article className="boundaryCard futureBoundary">
            <div className="boundaryLabel"><span aria-hidden="true">◇</span> gate · before customers</div>
            <h3>Neon is the transactional boundary</h3>
            <p>
              Blob stores bytes. It cannot safely own tenants, quota reservations,
              entitlements, webhook deduplication, or multi-device conflicts.
            </p>
            <ul>
              <li>Real identity and workspace isolation</li>
              <li>Transactional quota and usage ledger</li>
              <li>Staged upload verification and repair</li>
            </ul>
          </article>
        </div>
      </section>

      <section className="businessSection" id="business-model" aria-labelledby="business-title">
        <div className="shell">
          <div className="sectionHeading splitHeading businessHeading">
            <div>
              <div className="eyebrow">Path to sustainability</div>
              <h2 id="business-title">Charge for the safety net.<br />Never the local tool.</h2>
            </div>
            <div className="hypothesisCopy">
              <span className="hypothesisBadge">Design hypothesis · not for sale</span>
              <p>
                The free CLI earns trust locally. A paid vault can fund durable remote
                storage, verified restores, and multi-device continuity without turning
                core recovery into a subscription hostage.
              </p>
            </div>
          </div>

          <div className="pricingGrid" aria-label="Future pricing hypothesis">
            <article className="priceCard">
              <div className="priceHeader"><h3>Local</h3><small>available now</small></div>
              <div className="price"><strong>$0</strong><span>local core</span></div>
              <p>The complete local-first workflow remains useful without an account.</p>
              <ul>
                <li>Save, restore, search, and diff</li>
                <li>SQLite and vector indexes on-device</li>
                <li>CLI, Studio, MCP, and docs</li>
              </ul>
            </article>
            <article className="priceCard featuredPrice">
              <div className="priceHeader"><h3>Remote Vault Beta</h3><small>hypothesis</small></div>
              <div className="price"><strong>$15</strong><span>/ month</span></div>
              <p>A recovery hypothesis sized to test first-upload verification economics.</p>
              <ul>
                <li>50 GB remote storage</li>
                <li>5 GB download allowance / month · metering TBD</li>
                <li>3 authorized devices</li>
              </ul>
            </article>
            <article className="priceCard quietPrice">
              <div className="priceHeader"><h3>Teams</h3><small>later</small></div>
              <div className="price"><strong>—</strong><span>after demand</span></div>
              <p>Shared custody requires a different security and collaboration model.</p>
              <ul>
                <li>Workspace roles and audit history</li>
                <li>Policy-managed retention</li>
                <li>Consolidated billing</li>
              </ul>
            </article>
          </div>

          <div className="billingBlueprint" aria-labelledby="billing-flow-title">
            <div className="billingIntro">
              <span className="eyebrow">Future Stripe boundary</span>
              <h3 id="billing-flow-title">Payment records intent.<br />Entitlement policy grants access.</h3>
              <p>
                A successful browser redirect is never an entitlement. Only verified
                webhook delivery or reconciled canonical Stripe state, persisted in Neon,
                can update the internal access projection read by an authorized device.
              </p>
            </div>
            <ol className="billingFlow">
              {billingFlow.map(([title, copy], index) => (
                <li key={title}>
                  <span>{String(index + 1).padStart(2, "0")}</span>
                  <div><strong>{title}</strong><small>{copy}</small></div>
                </li>
              ))}
            </ol>
            <div className="recoveryPromise">
              <span aria-hidden="true">↗</span>
              <p><strong>Recovery-first billing:</strong> payment trouble may pause new uploads after a grace period. Existing recovery and export remain available through a clearly documented retention window before any tombstone lifecycle.</p>
            </div>
          </div>
        </div>
      </section>

      <footer className="shell footer">
        <span>Branch experiment · never deployed</span>
        <div>
          <a href="#recovery-lab">Back to lab ↑</a>
          <a href="https://file.cheap/docs/">Local-first docs ↗</a>
        </div>
      </footer>
    </main>
  );
}
