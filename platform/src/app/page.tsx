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
    command: "fcheap save ./checkout-repro --tool agent",
    copy: "Copy any file or directory into the local vault. Its manifest records source, tool, tags, sizes, hashes, and likely secret findings.",
    number: "01",
    signal: "snapshot",
    title: "Save the whole thing",
  },
  {
    command: "fcheap analyze 8f3a91c2",
    copy: "Index readable files only when you need retrieval. Keyword search stays local; semantic and hybrid modes remain explicit options.",
    number: "02",
    signal: "index",
    title: "Make the useful parts findable",
  },
  {
    command: 'fcheap search "checkout timeout"',
    copy: "Search across saved files and inspect provenance first. A result is a ranked lead into the evidence, never a replacement for it.",
    number: "03",
    signal: "retrieve",
    title: "Follow the evidence back",
  },
  {
    command: "fcheap restore 8f3a91c2 --to ./recovered",
    copy: "Restore into a clean directory. Every recovered file is checked against the manifest before file.cheap calls the restore complete.",
    number: "04",
    signal: "verify",
    title: "Recover exact bytes",
  },
] as const;

const artifactKinds = [
  "screenshots",
  "logs",
  "reports",
  "traces",
  "repro folders",
  "generated bundles",
] as const;

const handoffSteps = [
  {
    copy: "Finish a native run pack before its snapshot is taken.",
    number: "01",
    title: "Cairntrace or Glyphrun",
  },
  {
    copy: "Save the complete pack and keep its immutable bytes local.",
    number: "02",
    title: "file.cheap",
  },
  {
    copy: "Emit a versioned, credential-free pointer to the saved stash.",
    number: "03",
    title: "ArtifactRefV1",
  },
  {
    copy: "For a matching Cairn run ID, attach the reference during first ingestion.",
    number: "04",
    title: "Optional Chalupa path",
  },
] as const;

export default function HomePage() {
  return (
    <div className="landingPage">
      <a className="skipLink" href="#main-content">
        Skip to content
      </a>

      <nav className="siteNav shell" aria-label="Primary navigation">
        <Link className="wordmark" href="/" aria-label="file.cheap home">
          <span className="wordmarkMark" aria-hidden="true">
            f/
          </span>
          <span>file.cheap</span>
        </Link>

        <div className="navLinks">
          <a className="navWorkflowLink" href="#workflow">
            How it works
          </a>
          <a className="navIntegrationLink" href="#integrations">
            Handoff
          </a>
          <a className="navOptionalLink" href="/guide/">
            Docs
          </a>
        </div>

        <Link className="navConsoleLink" href="/console">
          <span className="localPulse" aria-hidden="true" />
          Owner console
        </Link>
      </nav>

      <main id="main-content" tabIndex={-1}>
        <section className="hero shell" aria-labelledby="hero-title">
          <div className="heroRail" aria-hidden="true">
            <span>local artifact vault</span>
            <span>manifest / index / restore</span>
          </div>

          <div className="heroCopy">
            <div className="eyebrow">
              <span>CLI + stdio MCP</span>
              <span>your machine is the vault</span>
            </div>
            <h1 id="hero-title">
              <span>Agent work ends.</span>
              <span className="heroAccent">The files shouldn&apos;t.</span>
            </h1>
            <p className="heroLead">
              Keep the files your agents create. Save any file tree with provenance
              and hashes, find it locally, and restore the exact bytes when the chat
              that produced them is long gone.
            </p>
            <div className="heroActions">
              <a className="button primary" href="/guide/getting-started">
                Install file.cheap
                <span aria-hidden="true">↗</span>
              </a>
              <a
                className="button secondary"
                href="/integrations/local-artifact-references"
              >
                Trace an artifact handoff
              </a>
            </div>
            <div className="installLine" aria-label="Example install command">
              <span aria-hidden="true">$</span>
              <code>brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap</code>
            </div>
          </div>

          <aside className="artifactScene" aria-label="A saved local stash">
            <div className="folderBack" aria-hidden="true">
              <span>checkout-repro/</span>
            </div>
            <div className="terminalSheet">
              <div className="terminalBar">
                <span>~/work</span>
                <span>local session</span>
              </div>
              <div className="terminalBody">
                <p>
                  <span className="prompt">$</span> fcheap save ./checkout-repro
                </p>
                <p className="terminalMuted">scanning 38 files...</p>
                <p className="terminalMuted">checking likely secrets...</p>
                <p className="terminalSuccess">saved stash 8f3a91c2</p>
                <dl className="terminalFacts">
                  <div>
                    <dt>files</dt>
                    <dd>38</dd>
                  </div>
                  <div>
                    <dt>size</dt>
                    <dd>24.8 MB</dd>
                  </div>
                  <div>
                    <dt>index</dt>
                    <dd>ready</dd>
                  </div>
                </dl>
              </div>
            </div>

            <div className="manifestSlip">
              <div className="manifestHeading">
                <span>manifest.json</span>
                <span className="verifiedMark">verified</span>
              </div>
              <dl>
                <div>
                  <dt>stash</dt>
                  <dd>8f3a91c2</dd>
                </div>
                <div>
                  <dt>source</dt>
                  <dd>./checkout-repro</dd>
                </div>
                <div>
                  <dt>hashes</dt>
                  <dd>38 / 38</dd>
                </div>
                <div>
                  <dt>stored</dt>
                  <dd>local</dd>
                </div>
              </dl>
              <div className="manifestStamp" aria-hidden="true">
                bytes intact
              </div>
            </div>

            <p className="sceneCaption">
              The manifest is durable. The indexes are rebuildable. The source of
              truth stays on the machine that holds the evidence.
            </p>
          </aside>

          <dl className="heroFacts" aria-label="Product facts">
            <div>
              <dt>01 / local</dt>
              <dd>No account. No hosted vault.</dd>
            </div>
            <div>
              <dt>02 / inspectable</dt>
              <dd>Provenance travels with every stash.</dd>
            </div>
            <div>
              <dt>03 / verifiable</dt>
              <dd>SHA-256 checks every restored file.</dd>
            </div>
          </dl>
        </section>

        <section className="artifactTicker" aria-label="Artifacts file.cheap can preserve">
          <div className="tickerLabel">agent residue worth keeping</div>
          <div className="tickerItems">
            {artifactKinds.map((kind) => (
              <span key={kind}>{kind}</span>
            ))}
          </div>
        </section>

        <section className="workflowField" id="workflow" aria-labelledby="workflow-title">
          <div className="workflow shell">
            <header className="workflowIntro">
              <div className="eyebrow lightEyebrow">A complete artifact lifecycle</div>
              <h2 id="workflow-title">
                A chain of custody for temporary work.
              </h2>
              <p>
                Search is useful. Recovery is the point. file.cheap keeps the path
                from a passing output to the complete, verified artifact deliberately
                short.
              </p>
              <div className="workflowLegend" aria-label="Lifecycle guarantee">
                <span>manifest is authority</span>
                <span>indexes can rebuild</span>
              </div>
            </header>

            <ol className="workflowList">
              {workflowSteps.map((step) => (
                <li key={step.number}>
                  <div className="stepMeta">
                    <span>{step.number}</span>
                    <span>{step.signal}</span>
                  </div>
                  <div className="stepCopy">
                    <h3>{step.title}</h3>
                    <p>{step.copy}</p>
                  </div>
                  <code>{step.command}</code>
                </li>
              ))}
            </ol>
          </div>
        </section>

        <section
          className="integrationSection shell"
          id="integrations"
          aria-labelledby="integrations-title"
        >
          <header className="integrationHeading">
            <div>
              <div className="eyebrow">ArtifactRefV1 · local metadata handoff</div>
              <h2 id="integrations-title">
                The bytes stay put.
                <br />
                <span>The reference travels.</span>
              </h2>
            </div>
            <p>
              Keep one artifact and give each tool its own job. Producers create
              evidence; file.cheap snapshots and verifies it; other systems receive a
              portable reference instead of a copy of the bytes.
            </p>
          </header>

          <ol className="handoffMap" aria-label="Local artifact handoff">
            {handoffSteps.map((step) => (
              <li key={step.number}>
                <span className="handoffNumber">{step.number}</span>
                <div className="handoffNode">
                  <strong>{step.title}</strong>
                  <p>{step.copy}</p>
                </div>
                {step.number !== "04" ? (
                  <span className="handoffArrow" aria-hidden="true">
                    →
                  </span>
                ) : null}
              </li>
            ))}
          </ol>

          <aside className="integrationBoundary" aria-label="Integration boundary">
            <span className="boundaryTag">boundary note</span>
            <p>
              This is a metadata handoff, not a sync service. Chalupa does not receive
              artifact bytes, its report adapter ships separately, and an unresolved
              local reference is shown as unavailable—not as a failed run. Glyphrun
              references can also remain independent of Chalupa.
            </p>
            <a href="/integrations/local-artifact-references">
              Read the handoff contract
              <span aria-hidden="true">↗</span>
            </a>
          </aside>
        </section>

        <section className="operatorsSection shell" id="agents" aria-labelledby="operators-title">
          <header>
            <div className="eyebrow">Two ways into the same vault</div>
            <h2 id="operators-title">Legible to people. Operable by agents.</h2>
          </header>

          <div className="operatorSplit">
            <article>
              <div className="operatorMarker" aria-hidden="true">
                01
              </div>
              <div>
                <span className="operatorLabel">people / CLI + Studio</span>
                <h3>See what is saved before you touch it.</h3>
                <p>
                  Browse manifests, compare a stash with current files, and choose
                  exactly when an operation may write or delete.
                </p>
                <ul>
                  <li>Local SQLite and search indexes</li>
                  <li>Streaming compression and verified restore</li>
                  <li>Explicit retention and cleanup decisions</li>
                </ul>
              </div>
            </article>

            <article>
              <div className="operatorMarker" aria-hidden="true">
                02
              </div>
              <div>
                <span className="operatorLabel">agents / stdio MCP</span>
                <h3>Give agents a filing system, not storage folklore.</h3>
                <p>
                  Typed tools, resources, prompts, and a version-matched guide let
                  compatible MCP clients preserve and retrieve evidence safely.
                </p>
                <ul>
                  <li>Fifteen typed local tools</li>
                  <li>Queryable manifests and bounded search results</li>
                  <li>Safety guidance available inside the protocol</li>
                </ul>
              </div>
            </article>
          </div>
        </section>

        <section className="finalCall">
          <div className="shell finalCallInner">
            <div>
              <span className="finalIndex">/ stash something worth finding</span>
              <h2>Temporary work deserves a return path.</h2>
            </div>
            <div className="finalActions">
              <a className="button finalButton" href="/guide/getting-started">
                Make your first stash
                <span aria-hidden="true">↗</span>
              </a>
              <p>Local-first core. Open source. No account required.</p>
            </div>
          </div>
        </section>
      </main>

      <footer className="siteFooter">
        <div className="shell footerInner">
          <span>file.cheap / local artifact vault</span>
          <div>
            <a href="/guide/">Documentation</a>
            <a href="/integrations/local-artifact-references">Integrations</a>
            <a href="mailto:hello@file.cheap">Email</a>
            <a href="https://github.com/abdul-hamid-achik/file.cheap">Source</a>
          </div>
          <span>built for the files between commits</span>
        </div>
      </footer>
    </div>
  );
}
