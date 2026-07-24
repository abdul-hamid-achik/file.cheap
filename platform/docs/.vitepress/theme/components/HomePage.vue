<script setup lang="ts">
import CopyButton from './CopyButton.vue'
import HomeCta from './HomeCta.vue'
import HomeFeatures from './HomeFeatures.vue'
import HomeInstall from './HomeInstall.vue'
import HomeMcp from './HomeMcp.vue'
import HomeStats from './HomeStats.vue'
import HomeTerminal from './HomeTerminal.vue'
import HomeWorkflow from './HomeWorkflow.vue'

const quickInstall = 'brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap'
</script>

<template>
  <main class="fc-home">
    <section class="fc-hero" aria-labelledby="fc-hero-title">
      <div class="fc-hero-copy">
        <p class="fc-kicker">
          <span>Local artifact vault for coding agents</span>
          <span>MCP-native</span>
          <span>Open source</span>
        </p>

        <p class="fc-wordmark" aria-hidden="true">file<span>.</span>cheap</p>
        <h1 id="fc-hero-title">Keep the files your agents create. Find them when they matter.</h1>
        <p class="fc-hero-lede">
          Snapshot screenshots, logs, reports, transcripts, and repro bundles with
          provenance. Search their contents, then restore every file with SHA-256
          verification—from your terminal or through MCP.
        </p>

        <div class="fc-hero-actions">
          <a class="fc-button fc-button-primary" href="/guide/getting-started">
            Install file.cheap
            <span aria-hidden="true">→</span>
          </a>
          <a class="fc-button fc-button-secondary" href="/guide/agent-guide">
            Connect an agent
          </a>
        </div>

        <div class="fc-quick-install" aria-label="Quick install command">
          <span class="fc-quick-label">macOS</span>
          <code>{{ quickInstall }}</code>
          <CopyButton :text="quickInstall" label="Copy macOS install command" />
        </div>

        <p class="fc-boundary-note">
          No account or hosted service required. The vault and keyword index stay on
          your machine; remote semantic search is optional and explicitly configured.
        </p>
      </div>

      <HomeTerminal />
    </section>

    <HomeStats />
    <HomeFeatures />
    <HomeWorkflow />
    <HomeMcp />
    <HomeCta />
    <HomeInstall />
  </main>
</template>

<style scoped>
.fc-home {
  overflow: clip;
}

.fc-hero {
  width: min(100% - 3rem, 1240px);
  min-height: calc(100dvh - var(--vp-nav-height));
  margin: 0 auto;
  padding: clamp(5rem, 9vw, 8.5rem) 0 clamp(4rem, 7vw, 6.5rem);
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(480px, 1.05fr);
  align-items: center;
  gap: clamp(3rem, 7vw, 7rem);
  position: relative;
}

.fc-hero::before {
  content: '';
  position: absolute;
  width: 22rem;
  height: 22rem;
  top: 8%;
  left: -18rem;
  border: 1px solid var(--fc-line);
  border-radius: 50%;
  box-shadow:
    0 0 0 4rem color-mix(in srgb, var(--fc-line) 36%, transparent),
    0 0 0 8rem color-mix(in srgb, var(--fc-line) 18%, transparent);
  pointer-events: none;
}

.fc-hero-copy {
  position: relative;
  z-index: 1;
}

.fc-kicker {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
  margin: 0 0 2rem;
}

.fc-kicker span {
  padding: 0.34rem 0.52rem;
  border: 1px solid var(--fc-line-strong);
  background: var(--fc-surface);
  color: var(--fc-ink-soft);
  font: 700 0.68rem/1 var(--fc-mono);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.fc-kicker span:first-child {
  border-color: var(--fc-green);
  color: var(--fc-green);
}

.fc-wordmark {
  margin: 0 0 0.85rem;
  color: var(--fc-accent);
  font: 750 clamp(1rem, 2vw, 1.3rem)/1 var(--fc-mono);
  letter-spacing: -0.06em;
}

.fc-wordmark span {
  display: inline-block;
  width: 0.34em;
  height: 0.34em;
  margin: 0 0.08em;
  background: currentColor;
  color: transparent;
  transform: rotate(8deg);
}

h1 {
  max-width: 11ch;
  margin: 0;
  color: var(--fc-ink);
  font: 720 clamp(3.35rem, 6.4vw, 6rem)/0.94 var(--fc-sans);
  letter-spacing: -0.065em;
  text-wrap: balance;
}

.fc-hero-lede {
  max-width: 58ch;
  margin: 1.8rem 0 0;
  color: var(--fc-ink-soft);
  font-size: clamp(1.08rem, 1.5vw, 1.24rem);
  line-height: 1.62;
}

.fc-hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.72rem;
  margin-top: 2rem;
}

.fc-quick-install {
  width: min(100%, 620px);
  margin-top: 2rem;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  border: 1px solid var(--fc-line-strong);
  background: var(--fc-code);
  color: var(--fc-code-ink);
}

.fc-quick-label {
  align-self: stretch;
  display: grid;
  place-items: center;
  padding: 0 0.8rem;
  border-right: 1px solid var(--fc-code-line);
  color: var(--fc-code-muted);
  font: 700 0.64rem/1 var(--fc-mono);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.fc-quick-install code {
  min-width: 0;
  padding: 0.82rem 0.9rem;
  overflow: hidden;
  color: inherit;
  font: 500 0.76rem/1.45 var(--fc-mono);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fc-boundary-note {
  max-width: 62ch;
  margin: 0.9rem 0 0;
  color: var(--fc-ink-muted);
  font-size: 0.78rem;
  line-height: 1.5;
}

@media (max-width: 980px) {
  .fc-hero {
    min-height: auto;
    grid-template-columns: 1fr;
    gap: 3.5rem;
    padding-top: 4.5rem;
  }

  h1 {
    max-width: 12ch;
  }
}

@media (max-width: 640px) {
  .fc-hero {
    width: min(100% - 2rem, 1240px);
    padding-top: 3.5rem;
  }

  h1 {
    font-size: clamp(3rem, 14vw, 4.4rem);
  }

  .fc-hero-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .fc-hero-actions .fc-button {
    justify-content: center;
  }

  .fc-quick-install {
    grid-template-columns: 1fr auto;
  }

  .fc-quick-label {
    display: none;
  }
}
</style>
