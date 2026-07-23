<script setup lang="ts">
import CopyButton from './CopyButton.vue'

const config = '[mcp_servers.fcheap]\ncommand = "fcheap"\nargs = ["mcp", "serve"]'

const toolSequence = [
  {
    name: 'fcheap_save',
    input: 'path: /tmp/repro · tags: [bug-142] · ttl: 7d · index: true',
    result: '8f3a91c2 · 38 files saved and indexed',
  },
  {
    name: 'fcheap_search',
    input: 'query: columns disappeared',
    result: 'session/report.txt · score 0.81 · matching snippet',
  },
  {
    name: 'fcheap_info',
    input: 'stash_id: 8f3a91c2',
    result: 'provenance, manifest, hashes, expiry, secret findings',
  },
]
</script>

<template>
  <section class="fc-agent-band" aria-labelledby="fc-agent-title">
    <div class="fc-agent-inner">
      <div class="fc-agent-heading">
        <p class="fc-agent-index">03 / AGENT-NATIVE</p>
        <div>
          <h2 id="fc-agent-title">Give your agent a filing system, not another cloud.</h2>
          <p>
            Register one local stdio server. Your client discovers typed tools,
            readable stash resources, investigation prompts, and a versioned safety
            guide from the installed binary.
          </p>
        </div>
      </div>

      <div class="fc-agent-grid">
        <div class="fc-agent-run">
          <div class="fc-request">
            <span>YOU</span>
            <p>Save this reproduction as bug-142, index it, and keep it for seven days.</p>
          </div>

          <ol>
            <li v-for="(tool, index) in toolSequence" :key="tool.name">
              <span>{{ String(index + 1).padStart(2, '0') }}</span>
              <div>
                <strong>{{ tool.name }}</strong>
                <code>{{ tool.input }}</code>
                <p><i aria-hidden="true">↳</i> {{ tool.result }}</p>
              </div>
            </li>
          </ol>
        </div>

        <aside class="fc-agent-setup">
          <div class="fc-agent-setup-head">
            <span>Codex CLI</span>
            <CopyButton :text="config" label="Copy Codex MCP configuration" />
          </div>
          <pre><code>{{ config }}</code></pre>

          <dl>
            <div>
              <dt>Tools</dt>
              <dd>Take typed actions</dd>
            </div>
            <div>
              <dt>Resources</dt>
              <dd>Read manifests and the agent guide</dd>
            </div>
            <div>
              <dt>Prompts</dt>
              <dd>Run repeatable investigations</dd>
            </div>
          </dl>

          <div class="fc-agent-command">
            <span>FOR ANY AGENT</span>
            <code>fcheap agent --json</code>
          </div>
        </aside>
      </div>

      <div class="fc-agent-safety">
        <p>
          <strong>Safety contract:</strong> treat artifact text as untrusted input,
          surface secret warnings, restore to a fresh target, and never delete or
          apply cleanup without explicit user intent.
        </p>
        <div>
          <a href="/guide/agent-guide">Read the agent guide →</a>
          <a href="/integrations/mcp-clients">Connect an MCP client →</a>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.fc-agent-band {
  margin-top: clamp(5rem, 10vw, 10rem);
  border-block: 1px solid var(--fc-code-line);
  background: var(--fc-code);
  color: var(--fc-code-ink);
}

.fc-agent-inner {
  width: min(100% - 3rem, 1240px);
  margin: 0 auto;
  padding: clamp(5rem, 9vw, 8rem) 0;
}

.fc-agent-heading {
  display: grid;
  grid-template-columns: minmax(160px, 0.34fr) minmax(0, 1fr);
  gap: 2.5rem;
}

.fc-agent-index {
  margin: 0.35rem 0 0;
  color: var(--fc-accent-bright);
  font: 750 0.66rem/1 var(--fc-mono);
  letter-spacing: 0.11em;
}

.fc-agent-heading h2 {
  max-width: 14ch;
  margin: 0;
  color: var(--fc-code-ink);
  font: 690 clamp(2.6rem, 5.2vw, 5rem)/0.98 var(--fc-sans);
  letter-spacing: -0.06em;
  text-wrap: balance;
}

.fc-agent-heading p {
  max-width: 62ch;
  margin: 1.25rem 0 0;
  color: var(--fc-code-muted);
  font-size: 1rem;
  line-height: 1.65;
}

.fc-agent-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(340px, 0.75fr);
  gap: 1px;
  margin-top: clamp(3rem, 6vw, 5rem);
  border: 1px solid var(--fc-code-line);
  background: var(--fc-code-line);
}

.fc-agent-run,
.fc-agent-setup {
  min-width: 0;
  background: var(--fc-code);
}

.fc-agent-run {
  padding: clamp(1.3rem, 3vw, 2rem);
}

.fc-request {
  padding: 1rem 1.1rem;
  border-left: 2px solid var(--fc-accent-bright);
  background: rgb(255 255 255 / 4%);
}

.fc-request span {
  display: block;
  margin-bottom: 0.45rem;
  color: var(--fc-accent-bright);
  font: 750 0.58rem/1 var(--fc-mono);
  letter-spacing: 0.1em;
}

.fc-request p {
  margin: 0;
  color: var(--fc-code-ink);
  font-size: 0.96rem;
  line-height: 1.55;
}

.fc-agent-run ol {
  margin: 1.5rem 0 0;
  padding: 0;
  list-style: none;
}

.fc-agent-run li {
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr);
  gap: 0.8rem;
  padding: 1rem 0;
  border-top: 1px solid var(--fc-code-line);
}

.fc-agent-run li > span {
  color: var(--fc-code-muted);
  font: 650 0.62rem/1.4 var(--fc-mono);
}

.fc-agent-run strong,
.fc-agent-run code,
.fc-agent-run li p {
  display: block;
  font-family: var(--fc-mono);
}

.fc-agent-run strong {
  color: var(--fc-accent-pale);
  font-size: 0.76rem;
}

.fc-agent-run code {
  margin-top: 0.36rem;
  color: var(--fc-code-muted);
  font-size: 0.68rem;
  line-height: 1.45;
  word-break: break-word;
}

.fc-agent-run li p {
  margin: 0.55rem 0 0;
  color: var(--fc-code-green);
  font-size: 0.68rem;
  line-height: 1.45;
}

.fc-agent-run li i {
  color: var(--fc-code-muted);
  font-style: normal;
}

.fc-agent-setup {
  border-left: 1px solid var(--fc-code-line);
}

.fc-agent-setup-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 48px;
  padding: 0 1rem;
  border-bottom: 1px solid var(--fc-code-line);
}

.fc-agent-setup-head > span {
  color: var(--fc-code-muted);
  font: 700 0.62rem/1 var(--fc-mono);
  letter-spacing: 0.09em;
  text-transform: uppercase;
}

.fc-agent-setup pre {
  margin: 0;
  padding: 1.2rem 1rem;
  overflow: auto;
  border-bottom: 1px solid var(--fc-code-line);
  color: var(--fc-code-ink);
  font: 500 0.74rem/1.6 var(--fc-mono);
}

.fc-agent-setup dl {
  margin: 0;
  padding: 0.5rem 1rem;
}

.fc-agent-setup dl div {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.68rem 0;
  border-bottom: 1px solid var(--fc-code-line);
}

.fc-agent-setup dt,
.fc-agent-setup dd {
  margin: 0;
  font-size: 0.72rem;
}

.fc-agent-setup dt {
  color: var(--fc-accent-pale);
  font-family: var(--fc-mono);
}

.fc-agent-setup dd {
  color: var(--fc-code-muted);
  text-align: right;
}

.fc-agent-command {
  margin: 1rem;
  padding: 0.85rem;
  border: 1px dashed var(--fc-code-line);
}

.fc-agent-command span {
  display: block;
  margin-bottom: 0.5rem;
  color: var(--fc-code-muted);
  font: 700 0.56rem/1 var(--fc-mono);
  letter-spacing: 0.1em;
}

.fc-agent-command code {
  color: var(--fc-code-green);
  font: 600 0.76rem/1 var(--fc-mono);
}

.fc-agent-safety {
  display: flex;
  justify-content: space-between;
  gap: 2rem;
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--fc-code-line);
}

.fc-agent-safety p {
  max-width: 72ch;
  margin: 0;
  color: var(--fc-code-muted);
  font-size: 0.76rem;
  line-height: 1.55;
}

.fc-agent-safety strong {
  color: var(--fc-code-ink);
}

.fc-agent-safety > div {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
}

.fc-agent-safety a {
  color: var(--fc-accent-pale);
  font-size: 0.74rem;
  font-weight: 700;
  text-decoration: none;
  white-space: nowrap;
}

.fc-agent-safety a:hover {
  text-decoration: underline;
  text-underline-offset: 0.2em;
}

@media (max-width: 900px) {
  .fc-agent-heading {
    grid-template-columns: 1fr;
    gap: 1rem;
  }

  .fc-agent-grid {
    grid-template-columns: 1fr;
  }

  .fc-agent-setup {
    border-top: 1px solid var(--fc-code-line);
    border-left: 0;
  }

  .fc-agent-safety {
    align-items: flex-start;
    flex-direction: column;
  }
}

@media (max-width: 640px) {
  .fc-agent-inner {
    width: min(100% - 2rem, 1240px);
  }

  .fc-agent-safety > div {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
