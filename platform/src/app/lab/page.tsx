import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { RecoveryLab } from "@/features/sync/recovery-lab";
import { getConfig } from "@/shared/config/env";
import { isRecoveryLabEnabled } from "@/shared/config/recovery-lab-access";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  alternates: {
    canonical: "/lab",
  },
  description:
    "Controlled recovery experiment for the optional file.cheap remote vault protocol.",
  robots: {
    follow: false,
    index: false,
    noarchive: true,
    nosnippet: true,
  },
  title: "Experimental recovery lab",
};

const protocolSteps = [
  {
    copy: "The control plane validates development authorization, size, hash, and current catalog state before any archive moves.",
    number: "01",
    title: "Plan the transfer",
  },
  {
    copy: "The browser moves bytes through a short-lived grant. The local adapter verifies SHA-256; the Blob spike records only presence, size, and ETag.",
    number: "02",
    title: "Move test bytes",
  },
  {
    copy: "Commit binds the adapter evidence to a stash ID. This prototype has no user, tenant, quota, or durable transactional boundary.",
    number: "03",
    title: "Commit the evidence",
  },
  {
    copy: "Download the complete archive, hash it again, and compare it with the portable recovery card before accepting the drill as successful.",
    number: "04",
    title: "Recover and prove",
  },
] as const;

export default function RecoveryLabPage() {
  if (!isRecoveryLabEnabled()) {
    notFound();
  }

  const config = getConfig();

  return (
    <main>
      <nav className="nav shell" aria-label="Primary navigation">
        <Link className="wordmark" href="/" aria-label="file.cheap home">
          <span className="wordmarkMark" aria-hidden="true">f·</span>
          <span>file.cheap</span>
        </Link>
        <div className="navLinks">
          <a href="#recovery-lab">Drill</a>
          <a className="navSecondaryLink" href="#protocol">Protocol</a>
          <a className="navOptionalLink" href="/guide/">Docs</a>
        </div>
        <div className="navMeta">
          <span className="statusDot" aria-hidden="true" />
          <span><span className="navMetaContext">controlled · </span>research lab</span>
        </div>
      </nav>

      <section className="hero shell" aria-labelledby="hero-title">
        <div className="heroCopy">
          <div className="eyebrow">Experimental remote recovery protocol</div>
          <h1 id="hero-title">
            Keep the local copy.<br />
            <em>Test the recovery proof.</em>
          </h1>
          <p className="heroLead">
            This gated, single-workspace laboratory exercises a possible remote
            extension to file.cheap. It is for synthetic, non-sensitive test data;
            it is not an account-backed vault or a production storage service.
          </p>
          <div className="heroActions">
            <a className="button primary" href="#recovery-lab">
              Run a controlled drill
            </a>
            <a className="button secondary" href="#protocol">
              Inspect the boundaries
            </a>
          </div>
          <dl className="heroFacts" aria-label="Laboratory facts">
            <div><dt>{config.storageDriver}</dt><dd>active adapter</dd></div>
            <div><dt>64 MiB</dt><dd>prototype limit</dd></div>
            <div><dt>zero</dt><dd>local files removed</dd></div>
          </dl>
        </div>

        <aside className="receiptScene" aria-label="Illustrative recovery receipt">
          <div className="receiptGlow" aria-hidden="true" />
          <div className="receiptCard">
            <div className="receiptTopline">
              <span>Illustrative drill report</span>
              <span className="verifiedBadge">
                <i aria-hidden="true">✓</i> example passed
              </span>
            </div>
            <div className="receiptTitle">
              <span className="receiptGlyph" aria-hidden="true">◫</span>
              <div>
                <strong>synthetic-session.fcheap</strong>
                <span>test archive · 18.4 MB</span>
              </div>
            </div>
            <div className="receiptHash">
              <span>SHA-256</span>
              <code>9f27 2b11 8a6e ··· 0e44 b31c</code>
            </div>
            <ol className="receiptTimeline">
              <li className="complete"><span>Test archive selected</span><b>01</b></li>
              <li className="complete"><span>Adapter evidence committed</span><b>02</b></li>
              <li className="complete"><span>Archive downloaded</span><b>03</b></li>
              <li className="complete"><span>Bytes matched locally</span><b>04</b></li>
            </ol>
            <div className="receiptFooter">
              <span>observation, not attestation</span>
              <span>protocol v1</span>
            </div>
          </div>
          <div className="receiptCaption">
            <span aria-hidden="true">↳</span>
            The drill report records what this browser observed. It is not a signed
            or tamper-evident server receipt.
          </div>
        </aside>
      </section>

      <section className="trustRail" aria-label="Experiment constraints">
        <div className="shell">
          <span>Synthetic data only</span>
          <span>Single workspace</span>
          <span>No local eviction</span>
          <span>Full recovery required</span>
        </div>
      </section>

      <section
        className="shell labSection"
        id="recovery-lab"
        aria-labelledby="lab-title"
      >
        <div className="sectionHeading splitHeading">
          <div>
            <div className="eyebrow">The complete recovery loop</div>
            <h2 id="lab-title">Trust the proof.<br />Not the progress bar.</h2>
          </div>
          <p>
            Use a small disposable archive, complete all five steps, and keep the
            generated recovery card beside the test stash manifest. Never enter a
            production credential or upload sensitive material.
          </p>
        </div>
        <RecoveryLab storageDriver={config.storageDriver} />
      </section>

      <section
        className="shell protocolSection"
        id="protocol"
        aria-labelledby="protocol-title"
      >
        <div className="sectionHeading">
          <div className="eyebrow">A deliberately narrow experiment</div>
          <h2 id="protocol-title">Four transitions.<br />Several hard gates.</h2>
          <p className="sectionLead">
            Remote bytes may extend a local workflow. They never grant permission
            to delete its source of truth, and this prototype does not establish
            production durability.
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
            <div className="boundaryLabel">
              <span aria-hidden="true">●</span> now · controlled prototype
            </div>
            <h3>Enough machinery to test recovery</h3>
            <p>
              The browser uses authenticated plans, bounded requests, direct transfer
              grants, portable recovery cards, and a complete hydrate-and-hash drill.
            </p>
            <ul>
              <li>Static development bearer credential</li>
              <li>One non-resumable transfer up to 64 MiB</li>
              <li>Adapter evidence recorded without local eviction</li>
            </ul>
          </article>
          <article className="boundaryCard futureBoundary">
            <div className="boundaryLabel">
              <span aria-hidden="true">◇</span> required · before external users
            </div>
            <h3>A real vault needs different boundaries</h3>
            <p>
              Blob storage alone cannot own identity, tenants, quotas, encryption,
              staged verification, repair, retention, or multi-device conflicts.
            </p>
            <ul>
              <li>Real authentication and workspace isolation</li>
              <li>Transactional catalog, quota, and entitlement state</li>
              <li>Staging, verification, quarantine, and repair</li>
              <li>Client-side encryption and recovery-key export</li>
            </ul>
          </article>
        </div>
      </section>

      <footer className="shell footer">
        <span>Controlled experiment · no production data</span>
        <div>
          <Link href="/">Public site</Link>
          <a href="/guide/">Local-first docs</a>
        </div>
      </footer>
    </main>
  );
}
