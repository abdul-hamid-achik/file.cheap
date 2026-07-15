<script setup lang="ts">
const proofs = [
  { value: 'files + folders', label: 'arbitrary artifacts' },
  { value: 'SHA-256', label: 'verified restores' },
  { value: 'BM25', label: 'local search built in' },
  { value: '14 tools', label: 'typed MCP surface' },
]
</script>

<template>
  <section class="fc-proof" aria-label="Product proof points">
    <p class="fc-proof-intro">
      <span>WHAT SHIPS TODAY</span>
      One CGO-free Go binary. No account. No background service.
    </p>
    <dl>
      <div v-for="proof in proofs" :key="proof.value">
        <dt>{{ proof.value }}</dt>
        <dd>{{ proof.label }}</dd>
      </div>
    </dl>
  </section>
</template>

<style scoped>
.fc-proof {
  width: min(100% - 3rem, 1240px);
  margin: 0 auto;
  display: grid;
  grid-template-columns: minmax(220px, 0.75fr) minmax(0, 2.25fr);
  border-block: 1px solid var(--fc-line-strong);
}

.fc-proof-intro {
  margin: 0;
  padding: 1.35rem 1.6rem 1.35rem 0;
  border-right: 1px solid var(--fc-line-strong);
  color: var(--fc-ink-soft);
  font-size: 0.86rem;
  line-height: 1.5;
}

.fc-proof-intro span {
  display: block;
  margin-bottom: 0.38rem;
  color: var(--fc-accent);
  font: 750 0.62rem/1 var(--fc-mono);
  letter-spacing: 0.11em;
}

dl {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  margin: 0;
}

dl div {
  min-width: 0;
  padding: 1.35rem 1rem;
  border-right: 1px solid var(--fc-line);
}

dl div:last-child {
  border-right: 0;
}

dt,
dd {
  margin: 0;
}

dt {
  color: var(--fc-ink);
  font: 750 clamp(0.95rem, 1.5vw, 1.12rem)/1.2 var(--fc-mono);
  letter-spacing: -0.04em;
}

dd {
  margin-top: 0.32rem;
  color: var(--fc-ink-muted);
  font-size: 0.72rem;
  line-height: 1.35;
}

@media (max-width: 900px) {
  .fc-proof {
    grid-template-columns: 1fr;
  }

  .fc-proof-intro {
    border-right: 0;
    border-bottom: 1px solid var(--fc-line-strong);
  }
}

@media (max-width: 640px) {
  .fc-proof {
    width: min(100% - 2rem, 1240px);
  }

  dl {
    grid-template-columns: repeat(2, 1fr);
  }

  dl div:nth-child(2) {
    border-right: 0;
  }

  dl div:nth-child(-n + 2) {
    border-bottom: 1px solid var(--fc-line);
  }
}
</style>
