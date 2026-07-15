<script setup lang="ts">
const jobs = [
  {
    number: '01',
    title: 'Capture the evidence',
    body: 'Copy any file tree into the vault. file.cheap records source, tool, tags, sizes, and hashes, then flags likely secrets before the artifact disappears into another temporary folder.',
    command: 'fcheap save ./repro --tag bug-142 --tool vidtrace --index',
    detail: 'payload + manifest + optional index',
    link: '/cli/save',
  },
  {
    number: '02',
    title: 'Find the exact file again',
    body: 'Filter by provenance or search one document per saved file. BM25 works locally with no model; semantic and hybrid modes are optional when wording is fuzzy.',
    command: 'fcheap search "columns disappeared after refresh"',
    detail: 'stash ID + path + score + snippet',
    link: '/cli/search',
  },
  {
    number: '03',
    title: 'Return it to useful work',
    body: 'Restore with hash verification, compare corresponding directory trees, or use optional vecgrep to rank source-code candidates related to saved evidence.',
    command: 'fcheap restore <stash-id>',
    detail: 'fresh target + integrity receipt',
    link: '/guide/workflows',
  },
]
</script>

<template>
  <section class="fc-section fc-evidence" aria-labelledby="fc-evidence-title">
    <div class="fc-section-heading">
      <p class="fc-section-index">01 / THE PROBLEM</p>
      <div>
        <h2 id="fc-evidence-title">Agent work leaves evidence everywhere.</h2>
        <p>
          Screenshots, logs, recordings, generated reports, and reproduction folders
          often outlive the conversation that created them. A stash gives each
          artifact a durable ID, provenance, and a route back to useful work.
        </p>
      </div>
    </div>

    <div class="fc-before-after">
      <div>
        <span>BEFORE</span>
        <code>/tmp/final-output-v7-really-final/</code>
        <p>No owner. No context. No reliable way back.</p>
      </div>
      <span class="fc-transfer" aria-hidden="true">→</span>
      <div>
        <span>AFTER</span>
        <code>stash://8f3a91c2 · bug-142 · vidtrace</code>
        <p>Named, searchable, restorable, and deliberately disposable.</p>
      </div>
    </div>

    <ol class="fc-job-list">
      <li v-for="job in jobs" :key="job.number">
        <span class="fc-job-number">{{ job.number }}</span>
        <div class="fc-job-copy">
          <h3>{{ job.title }}</h3>
          <p>{{ job.body }}</p>
        </div>
        <div class="fc-job-proof">
          <code>{{ job.command }}</code>
          <span>{{ job.detail }}</span>
          <a :href="job.link" :aria-label="'Read about ' + job.title">
            Read the workflow <span aria-hidden="true">→</span>
          </a>
        </div>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.fc-evidence {
  padding-top: clamp(6rem, 10vw, 10rem);
}

.fc-before-after {
  margin: clamp(3rem, 6vw, 5rem) 0 2.6rem;
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: stretch;
  border-block: 1px solid var(--fc-line-strong);
}

.fc-before-after > div {
  min-width: 0;
  padding: 1.4rem 0;
}

.fc-before-after > div:last-child {
  padding-left: 1.4rem;
}

.fc-before-after > div > span {
  display: block;
  margin-bottom: 0.7rem;
  color: var(--fc-ink-muted);
  font: 750 0.62rem/1 var(--fc-mono);
  letter-spacing: 0.12em;
}

.fc-before-after > div:last-child > span {
  color: var(--fc-green);
}

.fc-before-after code {
  display: block;
  overflow: hidden;
  color: var(--fc-ink);
  font: 650 clamp(0.75rem, 1.4vw, 0.92rem)/1.4 var(--fc-mono);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fc-before-after p {
  margin: 0.45rem 0 0;
  color: var(--fc-ink-muted);
  font-size: 0.82rem;
}

.fc-transfer {
  align-self: stretch;
  display: grid;
  place-items: center;
  width: 64px;
  border-inline: 1px solid var(--fc-line-strong);
  color: var(--fc-accent);
  font: 700 1.1rem/1 var(--fc-mono);
}

.fc-job-list {
  margin: 0;
  padding: 0;
  list-style: none;
  border-top: 1px solid var(--fc-line-strong);
}

.fc-job-list li {
  display: grid;
  grid-template-columns: 64px minmax(0, 1.15fr) minmax(320px, 0.85fr);
  gap: 1.6rem;
  padding: 2.2rem 0;
  border-bottom: 1px solid var(--fc-line-strong);
}

.fc-job-number {
  color: var(--fc-accent);
  font: 750 0.72rem/1 var(--fc-mono);
  letter-spacing: 0.08em;
}

.fc-job-copy h3 {
  margin: -0.15rem 0 0.6rem;
  color: var(--fc-ink);
  font: 680 clamp(1.45rem, 2.4vw, 2rem)/1.08 var(--fc-sans);
  letter-spacing: -0.045em;
}

.fc-job-copy p {
  max-width: 57ch;
  margin: 0;
  color: var(--fc-ink-soft);
  line-height: 1.65;
}

.fc-job-proof {
  align-self: start;
  display: flex;
  flex-direction: column;
  gap: 0.62rem;
}

.fc-job-proof code {
  padding: 0.78rem 0.85rem;
  border: 1px solid var(--fc-code-line);
  background: var(--fc-code);
  color: var(--fc-code-ink);
  font: 500 0.7rem/1.45 var(--fc-mono);
  word-break: break-word;
}

.fc-job-proof > span {
  color: var(--fc-ink-muted);
  font: 650 0.64rem/1 var(--fc-mono);
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.fc-job-proof a {
  width: fit-content;
  color: var(--fc-accent);
  font-size: 0.82rem;
  font-weight: 700;
  text-decoration: none;
}

.fc-job-proof a:hover {
  text-decoration: underline;
  text-underline-offset: 0.2em;
}

@media (max-width: 860px) {
  .fc-job-list li {
    grid-template-columns: 48px 1fr;
  }

  .fc-job-proof {
    grid-column: 2;
  }
}

@media (max-width: 640px) {
  .fc-before-after {
    grid-template-columns: 1fr;
  }

  .fc-before-after > div:last-child {
    padding-left: 0;
  }

  .fc-transfer {
    width: auto;
    height: 40px;
    border-block: 1px solid var(--fc-line-strong);
    border-inline: 0;
    transform: rotate(90deg);
  }

  .fc-job-list li {
    grid-template-columns: 1fr;
    gap: 1rem;
  }

  .fc-job-proof {
    grid-column: 1;
  }
}
</style>
