<script setup lang="ts">
import { computed } from 'vue'
import { useData, useRoute } from 'vitepress'

const { page } = useData()
const route = useRoute()

const sectionLabels: Record<string, string> = {
  cli: 'CLI',
  compare: 'Compare',
  guide: 'Guide',
  integrations: 'Integrations',
  learn: 'Learn',
  mcp: 'MCP',
  studio: 'Studio',
}

const sectionRoots: Record<string, string> = {
  cli: '/cli/',
  compare: '/compare/git-stash-worktree',
  guide: '/guide/',
  integrations: '/integrations/local-artifact-references',
  learn: '/learn',
  mcp: '/mcp/overview',
  studio: '/studio/overview',
}

const crumbs = computed(() => {
  const pathname = route.path.replace(/[?#].*$/, '').replace(/\/$/, '')
  const section = pathname.split('/').filter(Boolean)[0]
  const values = [{ label: 'file.cheap platform', href: '/' }]

  if (section) {
    values.push({
      label: sectionLabels[section] || section,
      href: sectionRoots[section] || `/${section}`,
    })
  }

  if (pathname !== sectionRoots[section]) {
    values.push({ label: page.value.title, href: pathname || '/' })
  }

  return values
})
</script>

<template>
  <nav class="fc-breadcrumbs" aria-label="Breadcrumb">
    <ol>
      <li v-for="(crumb, index) in crumbs" :key="crumb.href">
        <a
          v-if="index < crumbs.length - 1"
          :href="crumb.href"
          :target="crumb.href === '/' ? '_self' : undefined"
        >{{ crumb.label }}</a>
        <span v-else aria-current="page">{{ crumb.label }}</span>
      </li>
    </ol>
  </nav>
</template>

<style scoped>
.fc-breadcrumbs {
  margin: 0 0 1.4rem;
  font-family: var(--fc-mono);
  font-size: 0.72rem;
  color: var(--fc-ink-muted);
}

.fc-breadcrumbs ol {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.45rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.fc-breadcrumbs li {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  min-width: 0;
}

.fc-breadcrumbs li:not(:last-child)::after {
  content: '/';
  color: var(--fc-line-strong);
}

.fc-breadcrumbs a {
  color: var(--fc-ink-soft);
  text-decoration: none;
}

.fc-breadcrumbs a:hover {
  color: var(--fc-accent);
}

.fc-breadcrumbs span {
  overflow: hidden;
  max-width: 48ch;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
