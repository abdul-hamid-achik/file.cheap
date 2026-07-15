<template>
  <figure
    class="fc-artifact"
    aria-label="Example file.cheap workflow saving and searching a local agent artifact"
  >
    <div class="fc-artifact-label">
      <span>LOCAL VAULT</span>
      <span>EXAMPLE WORKFLOW</span>
      <span class="fc-live">READY</span>
    </div>

    <div class="fc-console">
      <div class="fc-console-bar">
        <span class="fc-console-mark" aria-hidden="true" />
        <span>~/projects/checkout</span>
        <span>fcheap</span>
      </div>

      <div class="fc-console-body">
        <p class="fc-console-command">
          <span>$</span>
          fcheap save ./agent-artifacts --tag bug-142 --index
        </p>
        <div class="fc-console-output">
          <p><strong>✓</strong> Saved stash: <b>8f3a91c2</b></p>
          <p><span>└─</span> 38 files · 4.2 MB · 38 indexed</p>
        </div>

        <p class="fc-console-command">
          <span>$</span>
          fcheap search "columns disappeared"
        </p>
        <div class="fc-search-hit">
          <span class="fc-hit-score">0.81</span>
          <div>
            <b>session/report.txt</b>
            <p>…table columns disappeared after refresh…</p>
          </div>
        </div>

        <p class="fc-console-command">
          <span>$</span>
          fcheap restore 8f3a91c2
        </p>
        <p class="fc-console-success">✓ Restored to /tmp/fcheap-8f3a91c2 · hashes verified</p>
      </div>
    </div>

    <figcaption class="fc-receipt">
      <div class="fc-receipt-head">
        <span>STASH RECEIPT</span>
        <strong>8f3a91c2</strong>
      </div>
      <dl>
        <div><dt>source</dt><dd>agent-artifacts/</dd></div>
        <div><dt>tag</dt><dd>bug-142</dd></div>
        <div><dt>files</dt><dd>38</dd></div>
        <div><dt>indexed</dt><dd>yes</dd></div>
        <div><dt>secrets</dt><dd class="fc-safe">0 found</dd></div>
        <div><dt>integrity</dt><dd>sha256</dd></div>
      </dl>
      <div class="fc-barcode" aria-hidden="true" />
      <p>MANIFEST IS THE SOURCE OF TRUTH</p>
    </figcaption>
  </figure>
</template>

<style scoped>
.fc-artifact {
  min-width: 0;
  margin: 0;
  position: relative;
  isolation: isolate;
}

.fc-artifact::before {
  content: '';
  position: absolute;
  inset: -1.4rem 3.5rem 1.5rem -1.5rem;
  border: 1px solid var(--fc-line);
  background:
    linear-gradient(var(--fc-grid-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--fc-grid-line) 1px, transparent 1px);
  background-size: 24px 24px;
  z-index: -1;
}

.fc-artifact-label {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin: 0 0 0.7rem 1.25rem;
  color: var(--fc-ink-muted);
  font: 700 0.62rem/1 var(--fc-mono);
  letter-spacing: 0.1em;
}

.fc-artifact-label span + span {
  padding-left: 0.6rem;
  border-left: 1px solid var(--fc-line-strong);
}

.fc-artifact-label .fc-live {
  margin-left: auto;
  color: var(--fc-green);
}

.fc-console {
  min-height: 470px;
  overflow: hidden;
  border: 1px solid var(--fc-code-line);
  background: var(--fc-code);
  box-shadow: 0 26px 70px -38px rgb(33 26 18 / 55%);
  color: var(--fc-code-ink);
}

.fc-console-bar {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 0.6rem;
  min-height: 42px;
  padding: 0 0.9rem;
  border-bottom: 1px solid var(--fc-code-line);
  color: var(--fc-code-muted);
  font: 600 0.64rem/1 var(--fc-mono);
  letter-spacing: 0.05em;
}

.fc-console-mark {
  width: 9px;
  height: 9px;
  border: 2px solid var(--fc-accent-bright);
  transform: rotate(8deg);
}

.fc-console-body {
  padding: 1.4rem 1.45rem 7.8rem;
  font: 500 0.75rem/1.65 var(--fc-mono);
}

.fc-console-command {
  margin: 0.9rem 0 0.55rem;
  color: var(--fc-code-ink);
}

.fc-console-command:first-child {
  margin-top: 0;
}

.fc-console-command span {
  margin-right: 0.55rem;
  color: var(--fc-accent-bright);
}

.fc-console-output {
  padding-left: 1.25rem;
  color: var(--fc-code-muted);
}

.fc-console-output p,
.fc-console-success,
.fc-search-hit p {
  margin: 0;
}

.fc-console-output strong,
.fc-console-success,
.fc-safe {
  color: var(--fc-code-green);
}

.fc-console-output b {
  color: var(--fc-accent-pale);
  font-weight: 600;
}

.fc-search-hit {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 0.8rem;
  margin-left: 1.25rem;
  padding: 0.72rem 0.8rem;
  border-left: 2px solid var(--fc-accent-bright);
  background: rgb(255 255 255 / 4%);
}

.fc-hit-score {
  color: var(--fc-accent-bright);
  font-weight: 700;
}

.fc-search-hit b {
  color: var(--fc-code-ink);
}

.fc-search-hit p {
  color: var(--fc-code-muted);
}

.fc-console-success {
  padding-left: 1.25rem;
}

.fc-receipt {
  width: min(310px, 66%);
  position: absolute;
  right: -1.5rem;
  bottom: -2.2rem;
  padding: 1rem 1rem 0.85rem;
  border: 1px solid var(--fc-receipt-line);
  background: var(--fc-receipt);
  box-shadow: 0 18px 50px -30px rgb(20 18 12 / 70%);
  color: var(--fc-receipt-ink);
  transform: rotate(-1.4deg);
}

.fc-receipt::before,
.fc-receipt::after {
  content: '';
  position: absolute;
  top: -6px;
  width: 32px;
  height: 13px;
  background: color-mix(in srgb, var(--fc-surface) 80%, transparent);
  border: 1px solid var(--fc-line);
  transform: rotate(-6deg);
}

.fc-receipt::before { left: 20px; }
.fc-receipt::after { right: 24px; transform: rotate(7deg); }

.fc-receipt-head {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  padding-bottom: 0.65rem;
  border-bottom: 1px dashed var(--fc-receipt-line);
  font: 750 0.62rem/1 var(--fc-mono);
  letter-spacing: 0.08em;
}

.fc-receipt dl {
  display: grid;
  gap: 0.3rem;
  margin: 0.7rem 0;
}

.fc-receipt dl div {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
}

.fc-receipt dt,
.fc-receipt dd {
  margin: 0;
  font: 600 0.64rem/1.35 var(--fc-mono);
}

.fc-receipt dt {
  color: var(--fc-receipt-muted);
  text-transform: uppercase;
}

.fc-barcode {
  height: 24px;
  margin: 0.65rem 0 0.45rem;
  background: repeating-linear-gradient(
    90deg,
    currentColor 0 2px,
    transparent 2px 4px,
    currentColor 4px 5px,
    transparent 5px 8px
  );
}

.fc-receipt > p {
  margin: 0;
  text-align: center;
  font: 700 0.5rem/1 var(--fc-mono);
  letter-spacing: 0.12em;
}

@media (max-width: 640px) {
  .fc-artifact::before {
    inset: -1rem 1rem 1.5rem -1rem;
  }

  .fc-console {
    min-height: 440px;
  }

  .fc-console-body {
    padding: 1.15rem 1rem 7.6rem;
    font-size: 0.68rem;
  }

  .fc-receipt {
    width: 72%;
    right: -0.4rem;
  }
}
</style>
