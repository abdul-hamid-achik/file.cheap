<script setup lang="ts">
const boundaries = [
  {
    label: 'Always local',
    tone: 'local',
    items: ['stash payloads', 'manifest.json', 'SQLite metadata', 'BM25 index', 'MCP process'],
  },
  {
    label: 'Local when loopback',
    tone: 'loopback',
    items: ['Ollama embeddings', 'semantic queries'],
  },
  {
    label: 'Leaves the machine only when configured',
    tone: 'remote',
    items: ['OpenAI embeddings', 'non-loopback Ollama', 'your MCP client and model provider'],
  },
]
</script>

<template>
  <section class="fc-section fc-boundaries" aria-labelledby="fc-boundaries-title">
    <div class="fc-section-heading">
      <p class="fc-section-index">04 / TRUST BOUNDARY</p>
      <div>
        <h2 id="fc-boundaries-title">Local by default. Explicit when it is not.</h2>
        <p>
          “Local MCP server” describes where file.cheap runs—not necessarily where
          your agent’s model runs. The boundary stays understandable because each
          optional network path is a configuration choice.
        </p>
      </div>
    </div>

    <div class="fc-boundary-grid">
      <article v-for="boundary in boundaries" :key="boundary.label" :class="boundary.tone">
        <header>
          <span aria-hidden="true" />
          <h3>{{ boundary.label }}</h3>
        </header>
        <ul>
          <li v-for="item in boundary.items" :key="item">{{ item }}</li>
        </ul>
      </article>
    </div>

    <div class="fc-boundary-footer">
      <p>
        Secret scanning is a warning system, not proof that content is safe to send.
        file.cheap blocks remote indexing of a stash with known findings unless you
        explicitly opt in.
      </p>
      <a href="/guide/core-concepts">Understand the architecture →</a>
    </div>
  </section>
</template>

<style scoped>
.fc-boundaries {
  padding-top: clamp(6rem, 10vw, 10rem);
}

.fc-boundary-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1px;
  margin-top: clamp(3rem, 6vw, 5rem);
  border: 1px solid var(--fc-line-strong);
  background: var(--fc-line-strong);
}

.fc-boundary-grid article {
  min-width: 0;
  padding: 1.5rem;
  background: var(--fc-surface);
}

.fc-boundary-grid header {
  min-height: 3.5rem;
  display: grid;
  grid-template-columns: auto 1fr;
  align-items: start;
  gap: 0.72rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--fc-line);
}

.fc-boundary-grid header span {
  width: 9px;
  height: 9px;
  margin-top: 0.28rem;
  background: var(--fc-green);
  transform: rotate(8deg);
}

.fc-boundary-grid .loopback header span {
  background: var(--fc-accent);
}

.fc-boundary-grid .remote header span {
  background: transparent;
  border: 2px solid var(--fc-ink-muted);
}

.fc-boundary-grid h3 {
  margin: 0;
  color: var(--fc-ink);
  font: 680 1rem/1.3 var(--fc-sans);
  letter-spacing: -0.025em;
}

.fc-boundary-grid ul {
  margin: 1rem 0 0;
  padding: 0;
  list-style: none;
}

.fc-boundary-grid li {
  position: relative;
  padding: 0.48rem 0 0.48rem 1rem;
  color: var(--fc-ink-soft);
  font-size: 0.82rem;
  line-height: 1.4;
}

.fc-boundary-grid li::before {
  content: '—';
  position: absolute;
  left: 0;
  color: var(--fc-line-strong);
}

.fc-boundary-footer {
  display: flex;
  justify-content: space-between;
  gap: 2rem;
  padding: 1rem 0;
  border-bottom: 1px solid var(--fc-line-strong);
}

.fc-boundary-footer p {
  max-width: 76ch;
  margin: 0;
  color: var(--fc-ink-muted);
  font-size: 0.76rem;
  line-height: 1.55;
}

.fc-boundary-footer a {
  color: var(--fc-accent);
  font-size: 0.76rem;
  font-weight: 750;
  text-decoration: none;
  white-space: nowrap;
}

@media (max-width: 800px) {
  .fc-boundary-grid {
    grid-template-columns: 1fr;
  }

  .fc-boundary-grid header {
    min-height: 0;
  }

  .fc-boundary-footer {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
