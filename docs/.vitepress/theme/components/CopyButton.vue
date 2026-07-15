<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'

const props = defineProps<{
  text: string
  label?: string
}>()

const copied = ref(false)
const status = ref('')
let timer: ReturnType<typeof setTimeout> | undefined

function resetAfterDelay() {
  clearTimeout(timer)
  timer = setTimeout(() => {
    copied.value = false
    status.value = ''
  }, 1600)
}

async function copy() {
  try {
    await navigator.clipboard.writeText(props.text)
  } catch {
    status.value = 'Copy failed'
    resetAfterDelay()
    return
  }
  copied.value = true
  status.value = 'Copied to clipboard'
  resetAfterDelay()
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <button class="fc-copy" type="button" :aria-label="label || 'Copy to clipboard'" @click="copy">
    <span class="fc-copy-text"><slot /></span>
    <span class="fc-copy-icon" :class="{ done: copied }">
      <svg v-if="!copied" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <rect x="9" y="9" width="13" height="13" rx="2" />
        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
      </svg>
      <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polyline points="20 6 9 17 4 12" />
      </svg>
    </span>
  </button>
  <span class="fc-sr-only" role="status" aria-live="polite" aria-atomic="true">{{ status }}</span>
</template>

<style scoped>
.fc-copy {
  display: inline-flex;
  align-items: center;
  gap: 0.55rem;
  font-family: var(--fc-mono);
  font-size: 0.74rem;
  color: var(--fc-code-muted);
  background: transparent;
  border: 1px solid var(--fc-code-line);
  border-radius: 2px;
  padding: 0.45rem 0.55rem;
  cursor: pointer;
  transition: border-color 0.18s, color 0.18s, background 0.18s;
}
.fc-copy:hover {
  border-color: var(--fc-accent-bright);
  color: var(--fc-code-ink);
  background: rgb(255 255 255 / 4%);
}
.fc-copy-text { white-space: nowrap; }
.fc-copy-text:empty { display: none; }
.fc-copy-icon { display: inline-flex; color: currentColor; transition: color 0.18s; }
.fc-copy-icon.done { color: var(--fc-code-green); }
.fc-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>
