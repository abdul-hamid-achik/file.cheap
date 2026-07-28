import type { Metadata } from "next";
import Link from "next/link";

export const dynamic = "force-static";

export const metadata: Metadata = {
  alternates: {
    canonical: "/",
  },
  description:
    "Save agent-generated files locally with provenance and hashes, search them later, and restore verified bytes through the file.cheap CLI or MCP server.",
  openGraph: {
    description:
      "Save, find, and restore agent artifacts through one verified local-first lifecycle.",
    images: [
      {
        alt: "file.cheap — the local artifact vault for coding agents",
        height: 630,
        url: "/og.png",
        width: 1200,
      },
    ],
    title: "file.cheap — local artifact vault for coding agents",
    type: "website",
    url: "/",
  },
  robots: {
    follow: true,
    index: true,
  },
  title: "Local artifact vault for coding agents",
  twitter: {
    card: "summary_large_image",
    description:
      "Save, find, and restore agent artifacts through one verified local-first lifecycle.",
    images: ["/og.png"],
    title: "file.cheap — local artifact vault for coding agents",
  },
};

const workflowSteps = [
  {
    copy: "Copy any file or directory into the local vault. The manifest records its source, producing tool, tags, sizes, hashes, and likely secret findings.",
    number: "01",
    title: "Save the artifact",
  },
  {
    copy: "Index readable files when you need retrieval. Keyword search stays local; semantic and hybrid modes remain explicit optional choices.",
    number: "02",
    title: "Index what matters",
  },
  {
    copy: "Search across saved files and inspect provenance before restoring. Results are ranked leads, never a substitute for the complete artifact.",
    number: "03",
    title: "Find the evidence",
  },
  {
    copy: "Restore into a fresh directory and verify every recovered file against the hashes in its manifest before treating it as faithful evidence.",
    number: "04",
    title: "Restore and prove",
  },
] as const;

export default function HomePage() {
  return (
    <>
      <a className="skipLink" href="#main-content">Skip to content</a>
      <nav className="nav shell" aria-label="Primary navigation">
        <Link className="wordmark" href="/" aria-label="file.cheap home">
          <span className="wordmarkMark" aria-hidden="true">f·</span>
          <span>file.cheap</span>
        </Link>
        <div className="navLinks">
          <a className="navWorkflowLink" href="#workflow">Workflow</a>
          <a className="navIntegrationLink" href="#integrations">Integrations</a>
          <a className="navOptionalLink" href="/guide/">Docs</a>
          <Link className="navConsoleLink" href="/console">Owner console</Link>
        </div>
        <div className="navMeta">
          <span className="statusDot available" aria-hidden="true" />
          <span><span className="navMetaContext">available · </span>local core</span>
        </div>
      </nav>

      <main id="main-content" tabIndex={-1}>
      <section className="hero shell" aria-labelledby="hero-title">
        <div className="heroCopy">
          <div className="eyebrow">Local artifact vault for coding agents</div>
          <h1 id="hero-title">
            Keep the files your agents create.<br />
            <em>Find them when they matter.</em>
          </h1>
          <p className="heroLead">
            Save any file tree with provenance and hashes, search it locally, and
            restore the exact bytes later. file.cheap gives people, CLIs, and MCP
            clients one durable lifecycle on the machine that holds the evidence—no
            cloud account or hosted vault required.
          </p>
          <div className="heroActions">
            <a className="button primary" href="/guide/getting-started">
              Install the local CLI
            </a>
            <a className="button secondary" href="/integrations/local-artifact-references">
              See the artifact handoff
            </a>
          </div>
          <dl className="heroFacts" aria-label="Product facts">
            <div><dt>local</dt><dd>source of truth</dd></div>
            <div><dt>SHA-256</dt><dd>verified restore</dd></div>
            <div><dt>stdio MCP</dt><dd>agent-ready tools</dd></div>
          </dl>
        </div>

        <aside className="receiptScene" aria-label="Example local stash receipt">
          <div className="receiptGlow" aria-hidden="true" />
          <div className="receiptCard">
            <div className="receiptTopline">
              <span>Local stash manifest</span>
              <span className="verifiedBadge">
                <i aria-hidden="true">✓</i> hashes verified
              </span>
            </div>
            <div className="receiptTitle">
              <span className="receiptGlyph" aria-hidden="true">◫</span>
              <div>
                <strong>checkout-investigation</strong>
                <span>38 files · agent artifact</span>
              </div>
            </div>
            <div className="receiptHash">
              <span>stash ID</span>
              <code>8f3a 91c2 ··· local vault</code>
            </div>
            <ol className="receiptTimeline">
              <li className="complete"><span>Artifact saved</span><b>01</b></li>
              <li className="complete"><span>Readable files indexed</span><b>02</b></li>
              <li className="complete"><span>Evidence found by query</span><b>03</b></li>
              <li className="complete"><span>Restore hashes verified</span><b>04</b></li>
            </ol>
            <div className="receiptFooter">
              <span>manifest is durable</span>
              <span>indexes rebuild</span>
            </div>
          </div>
          <div className="receiptCaption">
            <span aria-hidden="true">↳</span>
            Search narrows the evidence. A verified restore recovers the complete
            saved bytes when an investigation needs more than a snippet.
          </div>
        </aside>
      </section>

      <section className="trustRail" aria-label="Product guarantees">
        <div className="shell">
          <span>No account required</span>
          <span>Local manifest authority</span>
          <span>Private keyword search</span>
          <span>Verified recovery</span>
        </div>
      </section>

      <section
        className="shell protocolSection"
        id="workflow"
        aria-labelledby="workflow-title"
      >
        <div className="sectionHeading">
          <div className="eyebrow">A complete artifact lifecycle</div>
          <h2 id="workflow-title">Save the work.<br />Recover the context.</h2>
          <p className="sectionLead">
            Temporary folders, traces, screenshots, reports, and generated bundles
            become named, searchable stashes with an explicit path back to their
            original bytes.
          </p>
        </div>
        <div className="protocolGrid">
          {workflowSteps.map((step) => (
            <article key={step.number}>
              <span className="stepNumber">{step.number}</span>
              <div className="protocolIcon" aria-hidden="true"><i /></div>
              <h3>{step.title}</h3>
              <p>{step.copy}</p>
            </article>
          ))}
        </div>

        <section className="integrationSection" id="integrations" aria-labelledby="integrations-title">
          <div className="integrationHeading">
            <div>
              <div className="eyebrow">ArtifactRefV1 · local metadata handoff</div>
              <h2 id="integrations-title">Keep one artifact. Give each tool its own job.</h2>
            </div>
            <p>
              Cairntrace and Glyphrun create evidence; file.cheap snapshots and
              verifies it. For artifacts associated with a Cairn run, a Chalupa
              report adapter can record the portable reference under that matching
              raw run ID. The bytes remain in the local vault.
            </p>
          </div>

          <ol className="handoffFlow" aria-label="Local artifact handoff">
            <li>
              <span className="handoffNumber">01</span>
              <strong>Cairntrace or Glyphrun</strong>
              <p>Finish a native run pack before any snapshot is taken.</p>
            </li>
            <li>
              <span className="handoffNumber">02</span>
              <strong>file.cheap</strong>
              <p>Save the completed pack, then emit its versioned local artifact reference.</p>
            </li>
            <li>
              <span className="handoffNumber">03</span>
              <strong>Optional Chalupa path</strong>
              <p>For a matching Cairn run ID, its separately deployed adapter attaches the complete reference during first ingestion.</p>
            </li>
            <li>
              <span className="handoffNumber">04</span>
              <strong>Matching local vault</strong>
              <p>An operator resolves and restores only where the stash exists.</p>
            </li>
          </ol>

          <aside className="integrationBoundary" aria-label="Integration boundary">
            <span className="integrationBoundaryMark" aria-hidden="true">↳</span>
            <p>
              This is a metadata handoff, not a sync service: Chalupa does not
              receive artifact bytes, its report adapter ships separately, and an
              unresolved local reference is shown as unavailable—not as a failed run.
              Glyphrun references can also remain independent of Chalupa.
            </p>
            <a href="/integrations/local-artifact-references">Read the local artifact reference guide</a>
          </aside>
        </section>

        <div className="boundaryGrid" id="agents">
          <article className="boundaryCard localBoundary">
            <div className="boundaryLabel">
              <span aria-hidden="true">●</span> people · CLI and Studio
            </div>
            <h3>A vault that stays understandable</h3>
            <p>
              Inspect manifests, browse stashes, compare a saved tree with current
              files, and choose exactly when an operation may write or delete.
            </p>
            <ul>
              <li>Local SQLite and search indexes</li>
              <li>Streaming compression and verified restore</li>
              <li>Explicit retention and cleanup decisions</li>
            </ul>
          </article>
          <article className="boundaryCard futureBoundary">
            <div className="boundaryLabel">
              <span aria-hidden="true">◇</span> agents · stdio MCP
            </div>
            <h3>A filing system agents can operate safely</h3>
            <p>
              Typed tools, resources, prompts, and a version-matched operating guide
              let any compatible MCP client preserve and retrieve artifacts without
              inventing storage behavior.
            </p>
            <ul>
              <li>Fifteen typed local tools</li>
              <li>Queryable manifests and bounded search results</li>
              <li>Safety guidance available inside the protocol</li>
            </ul>
          </article>
        </div>
      </section>

      <footer className="shell footer">
        <span>Local-first core · open source</span>
        <div>
          <a href="/guide/">Documentation</a>
          <a href="/integrations/local-artifact-references">Integrations</a>
          <a href="mailto:hello@file.cheap">Email</a>
          <a href="https://github.com/abdul-hamid-achik/file.cheap">
            Source
          </a>
        </div>
      </footer>
      </main>
    </>
  );
}
