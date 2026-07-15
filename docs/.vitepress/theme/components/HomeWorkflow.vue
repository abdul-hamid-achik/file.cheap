<script setup lang="ts">
const stages = [
  {
    number: 'A',
    verb: 'Save',
    result: 'immutable payload',
    body: 'Copy the artifact, record provenance, hash every file, and scan for likely secrets.',
    command: 'fcheap save ./artifact --index',
  },
  {
    number: 'B',
    verb: 'Find',
    result: 'path + snippet',
    body: 'Search exact language locally, or opt into semantic retrieval when meaning matters more than wording.',
    command: 'fcheap search "refresh failed"',
  },
  {
    number: 'C',
    verb: 'Use',
    result: 'verified evidence',
    body: 'Restore to a fresh directory, diff a matching tree, or ask vecgrep for likely source-code candidates.',
    command: 'fcheap restore <stash-id>',
  },
  {
    number: 'D',
    verb: 'Retire',
    result: 'explicit cleanup',
    body: 'Compress, tag for retention, assign a TTL, or deliberately apply sweep and cleanup when the evidence is expendable.',
    command: 'fcheap sweep --apply',
  },
]
</script>

<template>
  <section class="fc-section fc-lifecycle" aria-labelledby="fc-lifecycle-title">
    <div class="fc-section-heading">
      <p class="fc-section-index">02 / THE LIFECYCLE</p>
      <div>
        <h2 id="fc-lifecycle-title">Keep context from capture to cleanup.</h2>
        <p>
          A stash is more than a copied folder. Its manifest makes every step
          inspectable, and destructive lifecycle actions stay explicit.
        </p>
      </div>
    </div>

    <ol class="fc-stages">
      <li v-for="stage in stages" :key="stage.number">
        <div class="fc-stage-index">
          <span>{{ stage.number }}</span>
          <i aria-hidden="true" />
        </div>
        <div class="fc-stage-copy">
          <p>{{ stage.result }}</p>
          <h3>{{ stage.verb }}</h3>
          <span>{{ stage.body }}</span>
        </div>
        <code>{{ stage.command }}</code>
      </li>
    </ol>

    <div class="fc-lifecycle-note">
      <span>IMPORTANT</span>
      <p>
        A TTL marks a stash as expired; it does not delete anything by itself.
        Removal happens only when you explicitly apply <code>sweep</code>,
        <code>cleanup</code>, or <code>drop --force</code>.
      </p>
      <a href="/guide/workflows">See complete workflows →</a>
    </div>
  </section>
</template>

<style scoped>
.fc-lifecycle {
  padding-top: clamp(5rem, 9vw, 8rem);
}

.fc-stages {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  margin: clamp(3rem, 6vw, 5rem) 0 0;
  padding: 0;
  border-block: 1px solid var(--fc-line-strong);
  list-style: none;
}

.fc-stages li {
  min-width: 0;
  padding: 1.5rem 1.35rem 1.6rem;
  border-right: 1px solid var(--fc-line-strong);
}

.fc-stages li:first-child {
  padding-left: 0;
}

.fc-stages li:last-child {
  padding-right: 0;
  border-right: 0;
}

.fc-stage-index {
  display: flex;
  align-items: center;
  gap: 0.7rem;
}

.fc-stage-index span {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 27px;
  height: 27px;
  border: 1px solid var(--fc-accent);
  color: var(--fc-accent);
  font: 750 0.64rem/1 var(--fc-mono);
  transform: rotate(-2deg);
}

.fc-stage-index i {
  width: 100%;
  height: 1px;
  background: var(--fc-line-strong);
}

.fc-stage-copy > p {
  margin: 1.35rem 0 0.35rem;
  color: var(--fc-green);
  font: 750 0.6rem/1 var(--fc-mono);
  letter-spacing: 0.09em;
  text-transform: uppercase;
}

.fc-stage-copy h3 {
  margin: 0;
  color: var(--fc-ink);
  font: 680 clamp(1.5rem, 2.4vw, 2rem)/1 var(--fc-sans);
  letter-spacing: -0.045em;
}

.fc-stage-copy > span {
  display: block;
  min-height: 6.4em;
  margin-top: 0.72rem;
  color: var(--fc-ink-soft);
  font-size: 0.84rem;
  line-height: 1.58;
}

.fc-stages code {
  display: block;
  margin-top: 1.15rem;
  color: var(--fc-ink);
  font: 600 0.67rem/1.4 var(--fc-mono);
  word-break: break-word;
}

.fc-lifecycle-note {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 1rem;
  padding: 1rem 1.15rem;
  border-bottom: 1px solid var(--fc-line-strong);
  background: var(--fc-surface-muted);
}

.fc-lifecycle-note > span {
  padding: 0.3rem 0.42rem;
  background: var(--fc-accent);
  color: var(--fc-accent-contrast);
  font: 800 0.58rem/1 var(--fc-mono);
  letter-spacing: 0.1em;
}

.fc-lifecycle-note p {
  margin: 0;
  color: var(--fc-ink-soft);
  font-size: 0.78rem;
  line-height: 1.5;
}

.fc-lifecycle-note code {
  color: var(--fc-ink);
  font-family: var(--fc-mono);
  font-size: 0.72rem;
}

.fc-lifecycle-note a {
  color: var(--fc-accent);
  font-size: 0.76rem;
  font-weight: 750;
  text-decoration: none;
  white-space: nowrap;
}

@media (max-width: 920px) {
  .fc-stages {
    grid-template-columns: repeat(2, 1fr);
  }

  .fc-stages li:nth-child(2) {
    border-right: 0;
  }

  .fc-stages li:nth-child(-n + 2) {
    border-bottom: 1px solid var(--fc-line-strong);
  }

  .fc-stages li:nth-child(3) {
    padding-left: 0;
  }

  .fc-stage-copy > span {
    min-height: 4.8em;
  }
}

@media (max-width: 640px) {
  .fc-stages {
    grid-template-columns: 1fr;
  }

  .fc-stages li,
  .fc-stages li:first-child,
  .fc-stages li:nth-child(3) {
    padding: 1.4rem 0;
    border-right: 0;
    border-bottom: 1px solid var(--fc-line-strong);
  }

  .fc-stages li:last-child {
    border-bottom: 0;
  }

  .fc-stage-copy > span {
    min-height: 0;
  }

  .fc-lifecycle-note {
    grid-template-columns: 1fr;
  }
}
</style>
