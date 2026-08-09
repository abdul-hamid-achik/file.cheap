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

const artifactFiles = [
  { hash: "b66f…91a2", name: "screenshots/checkout.png", size: "1.8 MB" },
  { hash: "2d4a…0f31", name: "logs/browser.jsonl", size: "468 KB" },
  { hash: "883c…7ae4", name: "reports/summary.md", size: "14 KB" },
  { hash: "15ae…b011", name: "trace/timeline.json", size: "22.5 MB" },
] as const;

const artifactKinds = [
  "screenshots",
  "logs",
  "reports",
  "traces",
  "repro folders",
  "generated bundles",
] as const;

const workflowSteps = [
  {
    command: "fcheap save ./checkout-repro --tool agent",
    copy: "Snapshot the complete directory. The manifest records where it came from, what produced it, every file hash, and likely secret findings.",
    label: "intake",
    number: "01",
    result: "stash 8f3a91c2 / 38 files",
    title: "Give the mess an ID",
  },
  {
    command: "fcheap analyze 8f3a91c2",
    copy: "Build a local retrieval index only when you want one. The saved bytes and manifest remain authoritative if the index disappears.",
    label: "index",
    number: "02",
    result: "BM25 ready / vectors optional",
    title: "Index the readable parts",
  },
  {
    command: 'fcheap search "checkout timeout"',
    copy: "Search every indexed stash, then follow the result back to its source, producing tool, path, tags, and complete artifact.",
    label: "locate",
    number: "03",
    result: "14 leads / provenance intact",
    title: "Find the useful fragment",
  },
  {
    command: "fcheap restore 8f3a91c2 --to ./recovered",
    copy: "Recover into a clean directory. file.cheap verifies every restored file against the hashes captured at intake.",
    label: "prove",
    number: "04",
    result: "38 / 38 hashes match",
    title: "Get the exact bytes back",
  },
] as const;

const handoffSteps = [
  {
    copy: "Finish the native evidence pack before it is saved.",
    number: "01",
    title: "Cairntrace or Glyphrun",
  },
  {
    copy: "Snapshot the complete pack and keep its bytes local.",
    number: "02",
    title: "file.cheap",
  },
  {
    copy: "Emit a versioned reference with no embedded credential.",
    number: "03",
    title: "ArtifactRefV1",
  },
  {
    copy: "Attach it only when the matching Cairn run ID exists.",
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
            Lifecycle
          </a>
          <a className="navIntegrationLink" href="#integrations">
            Handoff
          </a>
          <a className="navOptionalLink" href="/guide/">
            Docs
          </a>
        </div>

        <Link className="navConsoleLink" href="/console">
          <span aria-hidden="true">[ local ]</span>
          Owner console
        </Link>
      </nav>

      <main id="main-content" tabIndex={-1}>
        <section className="heroStage" aria-labelledby="hero-title">
          <div className="hero shell">
            <div className="heroStatement">
              <div className="systemLabel">
                <span>Artifact retention system</span>
                <span>CLI / Studio / stdio MCP</span>
              </div>

              <h1 id="hero-title">
                <span>Not in Git.</span>
                <span>Not in chat.</span>
                <span className="heroSignal">Not gone.</span>
              </h1>

              <p className="heroLead">
                Keep the files your agents create between commits. file.cheap gives
                screenshots, logs, reports, traces, and temporary folders a local ID,
                searchable context, and a verified way back.
              </p>

              <div className="heroActions">
                <a className="button primary" href="/guide/getting-started">
                  Stash your first folder
                  <span aria-hidden="true">-&gt;</span>
                </a>
                <a
                  className="button secondary"
                  href="/integrations/local-artifact-references"
                >
                  Inspect the handoff
                </a>
              </div>

              <div className="installLine" aria-label="Example install command">
                <span>install / macOS</span>
                <code>brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap</code>
              </div>
            </div>

            <aside className="inventoryPanel" aria-label="Example local artifact inventory">
              <header className="inventoryHeader">
                <div>
                  <span className="inventoryKicker">artifact intake</span>
                  <strong>checkout-repro</strong>
                </div>
                <div className="inventoryStatus">
                  <span aria-hidden="true" />
                  stored locally
                </div>
              </header>

              <div className="inventoryMeta">
                <span>STASH / 8f3a91c2</span>
                <span>FILES / 38</span>
                <span>SIZE / 24.8 MB</span>
                <span>TOOL / coding-agent</span>
              </div>

              <div className="inventoryWorkspace">
                <section className="fileIndex" aria-labelledby="file-index-title">
                  <div className="paneHeading">
                    <h2 id="file-index-title">content/</h2>
                    <span>4 of 38 shown</span>
                  </div>
                  <div className="fileRows">
                    {artifactFiles.map((file, index) => (
                      <div className={index === 2 ? "fileRow selectedFile" : "fileRow"} key={file.name}>
                        <span className="fileNumber">{String(index + 1).padStart(2, "0")}</span>
                        <span className="fileName">{file.name}</span>
                        <span className="fileSize">{file.size}</span>
                        <code>{file.hash}</code>
                      </div>
                    ))}
                  </div>
                </section>

                <section className="manifestPane" aria-labelledby="manifest-title">
                  <div className="paneHeading">
                    <h2 id="manifest-title">manifest.json</h2>
                    <span>source of truth</span>
                  </div>
                  <pre aria-label="Manifest excerpt">
                    <code>
                      <span>{`{`}</span>
                      <span>{`  "id": "8f3a91c2",`}</span>
                      <span>{`  "source": "./checkout-repro",`}</span>
                      <span>{`  "files": 38,`}</span>
                      <span>{`  "indexed": true,`}</span>
                      <span>{`  "verified": true`}</span>
                      <span>{`}`}</span>
                    </code>
                  </pre>
                </section>
              </div>

              <footer className="verificationBar">
                <div>
                  <span>manifest</span>
                  <strong>WRITTEN</strong>
                </div>
                <div>
                  <span>hash check</span>
                  <strong>38 / 38 PASS</strong>
                </div>
                <div>
                  <span>restore</span>
                  <strong>READY</strong>
                </div>
              </footer>
            </aside>

            <dl className="heroFacts" aria-label="Product facts">
              <div>
                <dt>authority</dt>
                <dd>Local manifest + original bytes</dd>
              </div>
              <div>
                <dt>retrieval</dt>
                <dd>BM25, semantic, or hybrid</dd>
              </div>
              <div>
                <dt>recovery</dt>
                <dd>SHA-256 verified restore</dd>
              </div>
              <div>
                <dt>account</dt>
                <dd>Not required</dd>
              </div>
            </dl>
          </div>
        </section>

        <section className="artifactTicker" aria-label="Artifacts file.cheap can preserve">
          <span className="tickerLabel">the files between commits</span>
          <div className="tickerItems">
            {artifactKinds.map((kind) => (
              <span key={kind}>{kind}</span>
            ))}
          </div>
        </section>

        <section className="workflowSection" id="workflow" aria-labelledby="workflow-title">
          <div className="shell workflowShell">
            <header className="sectionHeader">
              <span className="sectionCode">01 / lifecycle</span>
              <h2 id="workflow-title">
                One command in.
                <br />
                The same bytes out.
              </h2>
              <p>
                Temporary output becomes an addressable artifact without turning
                your machine into a cloud client or hiding the recovery contract.
              </p>
            </header>

            <ol className="workflowLedger">
              {workflowSteps.map((step) => (
                <li key={step.number}>
                  <div className="ledgerIndex">
                    <span>{step.number}</span>
                    <span>{step.label}</span>
                  </div>
                  <div className="ledgerMain">
                    <h3>{step.title}</h3>
                    <p>{step.copy}</p>
                  </div>
                  <div className="ledgerCommand">
                    <code>$ {step.command}</code>
                    <span>{step.result}</span>
                  </div>
                </li>
              ))}
            </ol>
          </div>
        </section>

        <section className="proofSection" aria-labelledby="proof-title">
          <div className="shell proofShell">
            <header className="proofHeader">
              <span className="sectionCode">02 / retrieval versus recovery</span>
              <h2 id="proof-title">
                Search gets you close.
                <br />
                <span>Restore gets you the truth.</span>
              </h2>
            </header>

            <div className="proofWorkspace">
              <section className="searchProof" aria-labelledby="search-proof-title">
                <div className="proofPaneHeader">
                  <div>
                    <span>query</span>
                    <h3 id="search-proof-title">checkout timeout</h3>
                  </div>
                  <strong>14 leads</strong>
                </div>
                <ol>
                  <li>
                    <span className="resultScore">0.91</span>
                    <div>
                      <strong>reports/summary.md</strong>
                      <p>...request timed out after checkout session confirmation...</p>
                    </div>
                  </li>
                  <li>
                    <span className="resultScore">0.84</span>
                    <div>
                      <strong>logs/browser.jsonl</strong>
                      <p>...payment iframe did not settle before navigation...</p>
                    </div>
                  </li>
                  <li>
                    <span className="resultScore">0.77</span>
                    <div>
                      <strong>trace/timeline.json</strong>
                      <p>...event=timeout source=checkout duration_ms=30001...</p>
                    </div>
                  </li>
                </ol>
                <p className="paneFootnote">
                  Results are bounded leads into the saved artifact, not a claim that
                  the snippet is complete evidence.
                </p>
              </section>

              <section className="restoreProof" aria-labelledby="restore-proof-title">
                <div className="proofPaneHeader">
                  <div>
                    <span>verified restore</span>
                    <h3 id="restore-proof-title">./recovered/</h3>
                  </div>
                  <strong>PASS</strong>
                </div>
                <div className="restoreScore">
                  <strong>38</strong>
                  <span>/ 38 hashes matched</span>
                </div>
                <dl>
                  <div>
                    <dt>manifest ID</dt>
                    <dd>8f3a91c2</dd>
                  </div>
                  <div>
                    <dt>files written</dt>
                    <dd>38</dd>
                  </div>
                  <div>
                    <dt>mismatches</dt>
                    <dd>0</dd>
                  </div>
                  <div>
                    <dt>source stash</dt>
                    <dd>retained</dd>
                  </div>
                </dl>
                <code className="restoreCommand">
                  fcheap restore 8f3a91c2 --to ./recovered
                </code>
              </section>
            </div>
          </div>
        </section>

        <section
          className="integrationSection shell"
          id="integrations"
          aria-labelledby="integrations-title"
        >
          <header className="integrationHeader">
            <span className="sectionCode">03 / ArtifactRefV1</span>
            <h2 id="integrations-title">
              Move the pointer.
              <br />
              Not the bytes.
            </h2>
            <p>
              A credential-free reference lets tools acknowledge one artifact
              without inventing a sync service or copying its payload into every
              system that mentions it.
            </p>
          </header>

          <div className="handoffWorkspace">
            <pre className="referenceReceipt" aria-label="ArtifactRefV1 excerpt">
              <code>
                <span>{`{`}</span>
                <span>{`  "version": "v1",`}</span>
                <span>{`  "transport": "fcheap-local",`}</span>
                <span>{`  "artifact_id": "8f3a91c2",`}</span>
                <span>{`  "producer": "cairntrace",`}</span>
                <span>{`  "native_run_id": "run_0142"`}</span>
                <span>{`}`}</span>
              </code>
              <span className="referenceNote">reference only / zero artifact bytes</span>
            </pre>

            <ol className="handoffFlow" aria-label="Local artifact handoff">
              {handoffSteps.map((step) => (
                <li key={step.number}>
                  <span className="handoffNumber">{step.number}</span>
                  <div>
                    <strong>{step.title}</strong>
                    <p>{step.copy}</p>
                  </div>
                </li>
              ))}
            </ol>
          </div>

          <aside className="integrationBoundary" aria-label="Integration boundary">
            <span>boundary / explicit</span>
            <p>
              This is a metadata handoff, not a sync service. Chalupa does not receive
              artifact bytes, its report adapter ships separately, and an unresolved
              local reference is shown as unavailable—not as a failed run. Glyphrun
              references can also remain independent of Chalupa.
            </p>
            <a href="/integrations/local-artifact-references">
              Read the complete contract <span aria-hidden="true">-&gt;</span>
            </a>
          </aside>
        </section>

        <section className="operatorSection" id="agents" aria-labelledby="operator-title">
          <div className="shell operatorShell">
            <header>
              <span className="sectionCode">04 / surfaces</span>
              <h2 id="operator-title">One vault. Two operators.</h2>
            </header>

            <div className="operatorColumns">
              <article>
                <span className="operatorType">people / CLI + Studio</span>
                <h3>Inspect first. Decide second.</h3>
                <p>
                  Browse manifests, compare a stash with current files, and choose
                  exactly when an operation may write or delete.
                </p>
                <ul>
                  <li>Local SQLite and search indexes</li>
                  <li>Streaming compression and verified restore</li>
                  <li>Explicit retention and cleanup decisions</li>
                </ul>
              </article>

              <article>
                <span className="operatorType">agents / stdio MCP</span>
                <h3>A filing system with typed boundaries.</h3>
                <p>
                  Tools, resources, prompts, and a version-matched guide let
                  compatible MCP clients preserve and retrieve evidence safely.
                </p>
                <ul>
                  <li>Fifteen typed local tools</li>
                  <li>Queryable manifests and bounded search results</li>
                  <li>Safety guidance available inside the protocol</li>
                </ul>
              </article>
            </div>
          </div>
        </section>

        <section className="finalCall">
          <div className="shell finalCallInner">
            <span className="finalLabel">return address for temporary work</span>
            <h2>Give the next investigation somewhere to look.</h2>
            <div>
              <a className="button finalButton" href="/guide/getting-started">
                Make the first stash <span aria-hidden="true">-&gt;</span>
              </a>
              <p>Local-first core / open source / no account required</p>
            </div>
          </div>
        </section>
      </main>

      <footer className="siteFooter">
        <div className="shell footerInner">
          <span>file.cheap / the files between commits</span>
          <div>
            <a href="/guide/">Documentation</a>
            <a href="/integrations/local-artifact-references">Integrations</a>
            <a href="mailto:hello@file.cheap">Email</a>
            <a href="https://github.com/abdul-hamid-achik/file.cheap">Source</a>
          </div>
          <span>manifest is authority / indexes rebuild</span>
        </div>
      </footer>
    </div>
  );
}
