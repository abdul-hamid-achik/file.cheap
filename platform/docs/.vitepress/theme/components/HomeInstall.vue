<script setup lang="ts">
import { computed, ref } from 'vue'
import CopyButton from './CopyButton.vue'

const tabs = [
  {
    id: 'mac',
    label: 'macOS',
    command: 'brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap',
    note: 'Installs the release cask from the file.cheap Homebrew tap.',
  },
  {
    id: 'linux',
    label: 'Linux (deb)',
    command: [
      "tag=\"$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/abdul-hamid-achik/file.cheap/releases/latest)\"",
      'tag="$' + '{tag##*/}"; version="$' + '{tag#v}"',
      'curl -fLO "https://github.com/abdul-hamid-achik/file.cheap/releases/download/$' + '{tag}/fcheap_$' + '{version}_linux_amd64.deb"',
      'sudo dpkg -i "fcheap_$' + '{version}_linux_amd64.deb"',
    ].join('\n'),
    note: 'Resolves the current version before downloading the versioned amd64 package.',
  },
  {
    id: 'source',
    label: 'From source',
    command: 'go install github.com/abdul-hamid-achik/file.cheap/cmd/fcheap@latest',
    note: 'Requires Go 1.25.12 or newer and produces a CGO-free binary.',
  },
]

const activeIndex = ref(0)
const active = computed(() => tabs[activeIndex.value])
const buttons = ref<Array<HTMLButtonElement | null>>([])

function select(index: number, focus = false) {
  activeIndex.value = (index + tabs.length) % tabs.length
  if (focus) buttons.value[activeIndex.value]?.focus()
}

function setButton(element: unknown, index: number) {
  buttons.value[index] = element as HTMLButtonElement | null
}

function onKeydown(event: KeyboardEvent, index: number) {
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
    event.preventDefault()
    select(index + 1, true)
  } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
    event.preventDefault()
    select(index - 1, true)
  } else if (event.key === 'Home') {
    event.preventDefault()
    select(0, true)
  } else if (event.key === 'End') {
    event.preventDefault()
    select(tabs.length - 1, true)
  }
}
</script>

<template>
  <section class="fc-install" aria-labelledby="fc-install-title">
    <div class="fc-install-inner">
      <div class="fc-install-copy">
        <p class="fc-install-index">05 / START HERE</p>
        <h2 id="fc-install-title">
          Give your agent durable file memory—without adding a cloud account.
        </h2>
        <p>
          Install one binary, run <code>fcheap doctor</code>, and create your first
          indexed stash. Connect MCP only when you want the same workflow inside an
          agent conversation.
        </p>
        <div class="fc-install-actions">
          <a class="fc-button fc-button-primary" href="/guide/getting-started">
            Follow the five-minute guide <span aria-hidden="true">→</span>
          </a>
          <a class="fc-button fc-button-secondary" href="/integrations/mcp-clients">
            Connect via MCP
          </a>
        </div>
      </div>

      <div class="fc-installer">
        <div class="fc-installer-tabs" role="tablist" aria-label="Installation method">
          <button
            v-for="(tab, index) in tabs"
            :id="'fc-install-tab-' + tab.id"
            :key="tab.id"
            :ref="element => setButton(element, index)"
            type="button"
            role="tab"
            :aria-controls="'fc-install-panel-' + tab.id"
            :aria-selected="activeIndex === index"
            :tabindex="activeIndex === index ? 0 : -1"
            @click="select(index)"
            @keydown="onKeydown($event, index)"
          >
            {{ tab.label }}
          </button>
        </div>

        <div
          :id="'fc-install-panel-' + active.id"
          class="fc-installer-panel"
          role="tabpanel"
          :aria-labelledby="'fc-install-tab-' + active.id"
          tabindex="0"
        >
          <div class="fc-installer-head">
            <span>INSTALL / {{ active.label.toUpperCase() }}</span>
            <CopyButton :text="active.command" :label="'Copy ' + active.label + ' install command'" />
          </div>
          <pre><code>{{ active.command }}</code></pre>
          <p>{{ active.note }}</p>
        </div>

        <div class="fc-after-install">
          <span>NEXT</span>
          <code>fcheap doctor</code>
          <code>fcheap save ./artifact --index</code>
          <code>fcheap artifact-ref &lt;stash-id&gt; --json</code>
          <code>fcheap agent</code>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.fc-install {
  margin-top: clamp(5rem, 10vw, 10rem);
  border-top: 1px solid var(--fc-line-strong);
  background:
    linear-gradient(var(--fc-grid-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--fc-grid-line) 1px, transparent 1px),
    var(--fc-surface-muted);
  background-size: 28px 28px;
}

.fc-install-inner {
  width: min(100% - 3rem, 1240px);
  margin: 0 auto;
  padding: clamp(5rem, 9vw, 8rem) 0;
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(480px, 1.1fr);
  align-items: center;
  gap: clamp(3rem, 7vw, 7rem);
}

.fc-install-index {
  margin: 0 0 1.2rem;
  color: var(--fc-accent);
  font: 750 0.66rem/1 var(--fc-mono);
  letter-spacing: 0.11em;
}

.fc-install-copy h2 {
  max-width: 13ch;
  margin: 0;
  color: var(--fc-ink);
  font: 690 clamp(2.7rem, 5.1vw, 5rem)/0.98 var(--fc-sans);
  letter-spacing: -0.06em;
  text-wrap: balance;
}

.fc-install-copy > p:not(.fc-install-index) {
  max-width: 58ch;
  margin: 1.3rem 0 0;
  color: var(--fc-ink-soft);
  line-height: 1.65;
}

.fc-install-copy > p code {
  color: var(--fc-ink);
  font: 600 0.84em/1 var(--fc-mono);
}

.fc-install-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.7rem;
  margin-top: 1.8rem;
}

.fc-installer {
  border: 1px solid var(--fc-line-strong);
  background: var(--fc-surface);
  box-shadow: 0 28px 75px -45px rgb(30 24 16 / 45%);
}

.fc-installer-tabs {
  display: flex;
  border-bottom: 1px solid var(--fc-line-strong);
  background: var(--fc-surface-muted);
}

.fc-installer-tabs button {
  flex: 1;
  min-height: 46px;
  padding: 0.7rem;
  border: 0;
  border-right: 1px solid var(--fc-line);
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--fc-ink-muted);
  cursor: pointer;
  font: 700 0.68rem/1 var(--fc-mono);
}

.fc-installer-tabs button:last-child {
  border-right: 0;
}

.fc-installer-tabs button[aria-selected='true'] {
  border-bottom-color: var(--fc-accent);
  background: var(--fc-surface);
  color: var(--fc-accent);
}

.fc-installer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 48px;
  padding: 0 0.9rem;
  border-bottom: 1px solid var(--fc-code-line);
  background: var(--fc-code);
}

.fc-installer-head > span {
  color: var(--fc-code-muted);
  font: 700 0.6rem/1 var(--fc-mono);
  letter-spacing: 0.08em;
}

.fc-installer-panel pre {
  min-height: 150px;
  margin: 0;
  padding: 1.15rem 1rem;
  overflow: auto;
  background: var(--fc-code);
  color: var(--fc-code-ink);
  font: 500 0.7rem/1.65 var(--fc-mono);
  white-space: pre-wrap;
  word-break: break-word;
}

.fc-installer-panel > p {
  min-height: 3.8rem;
  margin: 0;
  padding: 0.8rem 1rem;
  border-top: 1px solid var(--fc-line);
  color: var(--fc-ink-muted);
  font-size: 0.74rem;
  line-height: 1.5;
}

.fc-after-install {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.55rem;
  padding: 0.8rem 1rem;
  border-top: 1px solid var(--fc-line-strong);
}

.fc-after-install span {
  margin-right: 0.2rem;
  color: var(--fc-green);
  font: 750 0.56rem/1 var(--fc-mono);
  letter-spacing: 0.09em;
}

.fc-after-install code {
  padding: 0.3rem 0.42rem;
  border: 1px solid var(--fc-line);
  background: var(--fc-surface-muted);
  color: var(--fc-ink);
  font: 600 0.61rem/1 var(--fc-mono);
}

@media (max-width: 980px) {
  .fc-install-inner {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .fc-install-inner {
    width: min(100% - 2rem, 1240px);
  }

  .fc-install-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .fc-install-actions .fc-button {
    justify-content: center;
  }

  .fc-installer-tabs {
    flex-direction: column;
  }

  .fc-installer-tabs button {
    border-right: 0;
    border-bottom: 1px solid var(--fc-line);
  }
}
</style>
