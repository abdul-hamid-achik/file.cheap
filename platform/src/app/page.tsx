import { RecoveryLab } from "@/features/sync/recovery-lab";
import { getSyncService } from "@/features/sync/factory";
import { getConfig } from "@/shared/config/env";

export const dynamic = "force-dynamic";

export default async function HomePage() {
  const stashes = await getSyncService().listStashes();
  const config = getConfig();

  return (
    <main>
      <nav className="nav shell" aria-label="Primary navigation">
        <a className="wordmark" href="https://file.cheap" aria-label="file.cheap home">
          <span className="wordmarkMark" aria-hidden="true">f·</span>
          <span>file.cheap</span>
        </a>
        <div className="navMeta">
          <span className="statusDot" aria-hidden="true" />
          local prototype
        </div>
      </nav>

      <section className="hero shell">
        <div className="eyebrow">Recovery lab / sync protocol v1</div>
        <h1>Your agent artifacts,<br /><em>recoverable byte for byte.</em></h1>
        <p className="heroLead">
          A working control-plane experiment for moving immutable file.cheap stashes
          off-device, restoring them elsewhere, and verifying the exact SHA-256 before
          a local copy could ever be removed.
        </p>
        <div className="heroFacts" aria-label="Prototype facts">
          <div><strong>{config.storageDriver}</strong><span>active adapter</span></div>
          <div><strong>{stashes.length}</strong><span>committed stashes</span></div>
          <div><strong>none</strong><span>database required here</span></div>
        </div>
      </section>

      <section className="shell workingGrid" aria-labelledby="lab-title">
        <div className="sectionIntro">
          <div className="eyebrow">The complete loop</div>
          <h2 id="lab-title">Plan. Transfer. Commit. Prove.</h2>
          <p>
            This browser lab uses the same provider-neutral HTTP grants a future Go
            client will consume. Keep test archives small; the production CLI will hash
            and transfer with streaming and multipart support.
          </p>
        </div>
        <RecoveryLab initialStashes={stashes} />
      </section>

      <section className="shell architecture" aria-labelledby="architecture-title">
        <div className="sectionIntro compact">
          <div className="eyebrow">Deliberate boundaries</div>
          <h2 id="architecture-title">Small enough to trust.</h2>
        </div>
        <div className="architectureRail">
          <article>
            <span className="step">01</span>
            <h3>Local remains truth</h3>
            <p>Save, search, diff, and restore stay offline. Cloud is explicit and optional.</p>
          </article>
          <article>
            <span className="step">02</span>
            <h3>Functions coordinate</h3>
            <p>Small authenticated JSON plans operations; archive bytes take signed direct paths.</p>
          </article>
          <article>
            <span className="step">03</span>
            <h3>Objects stay immutable</h3>
            <p>SHA-256-derived keys make retries safe and conflicting stash identities visible.</p>
          </article>
          <article>
            <span className="step">04</span>
            <h3>Recovery proves safety</h3>
            <p>An ETag is presence, not proof. A full hydrate and hash gates future eviction.</p>
          </article>
        </div>
      </section>

      <section className="shell neonGate" aria-labelledby="neon-title">
        <div>
          <div className="eyebrow">The Neon gate</div>
          <h2 id="neon-title">Not needed for this lab.<br />Required before customers.</h2>
        </div>
        <p>
          A single versioned catalog is enough to exercise one vault. Authentication,
          tenants, quota reservations, usage, idempotency, tombstones, billing, and
          multi-device conflict resolution need transactional Postgres. Blob is the byte
          store; it should never pretend to be that database.
        </p>
      </section>

      <footer className="shell footer">
        <span>Branch experiment · never deployed</span>
        <a href="https://file.cheap/docs/">Read the local-first docs ↗</a>
      </footer>
    </main>
  );
}
