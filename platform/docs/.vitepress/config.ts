import { defineConfig } from 'vitepress'

const SITE_URL = 'https://file.cheap'
const SITE_DESC =
  'file.cheap is a local-first CLI and MCP server that saves, searches, verifies, restores, and manages agent-generated files and folders.'
const SITE_TITLE = 'file.cheap — local artifact vault for coding agents'
const AUTHOR = {
  '@type': 'Person',
  '@id': 'https://github.com/abdul-hamid-achik#person',
  name: 'Abdul Hamid Achik',
  url: 'https://github.com/abdul-hamid-achik',
}

type PageMeta = {
  title: string
  description: string
  image?: '/og.png' | '/og-install.png' | '/og-mcp.png'
}

const PAGE_META: Record<string, PageMeta> = {
  'index.md': {
    title: SITE_TITLE,
    description: SITE_DESC,
  },
  'cli/index.md': {
    title: 'file.cheap CLI reference',
    description: 'Find every fcheap command, global flag, JSON convention, and task-oriented command group from one CLI reference hub.',
  },
  'cli/analyze.md': {
    title: 'fcheap analyze — index stashes for search',
    description: 'Index a local stash in veclite and optionally search it with BM25, semantic, or hybrid retrieval.',
  },
  'cli/artifact-ref.md': {
    title: 'fcheap artifact-ref — local stash references',
    description: 'Emit a versioned fcheap-local reference so agent tools can share metadata for an existing stash without copying its bytes.',
  },
  'cli/auth.md': {
    title: 'fcheap auth — private console login',
    description: 'Authorize a local CLI through an owner-email device flow without placing a reusable token in command arguments or output.',
  },
  'cli/publish.md': {
    title: 'fcheap publish — private artifact publication',
    description: 'Publish one bounded local file through the private artifact service without exposing transfer credentials.',
  },
  'cli/pull.md': {
    title: 'fcheap pull — verified artifact recovery',
    description: 'Download one owner-scoped cloud artifact through a short-lived direct transfer and verify its size and SHA-256 locally.',
  },
  'cli/cleanup.md': {
    title: 'fcheap cleanup — reclaim local stash space',
    description: 'Score, review, and safely remove expired, duplicate, orphaned, or regenerable local stashes.',
  },
  'cli/compress.md': {
    title: 'fcheap compress — shrink stashes with zstd',
    description: 'Compress a local stash with streaming tar and zstd or gzip while preserving verified restore behavior.',
  },
  'cli/completion.md': {
    title: 'fcheap completion — generate shell completions',
    description: 'Generate completion scripts for Bash, Zsh, Fish, or PowerShell and install them in the appropriate shell configuration.',
  },
  'cli/config.md': {
    title: 'fcheap config — configure the local vault',
    description: 'Configure the file.cheap stash directory, compression, logging, vecgrep, embedding, and retention settings.',
  },
  'cli/connect.md': {
    title: 'fcheap connect — trace artifacts to code',
    description: 'Use a stashed bug artifact to search a live codebase and surface the file and line most likely responsible.',
  },
  'cli/diff.md': {
    title: 'fcheap diff — compare stashes with files',
    description: 'Compare a saved stash with a live directory to find added, missing, and content-changed files.',
  },
  'cli/docs.md': {
    title: 'fcheap docs — read embedded documentation',
    description: 'List and read documentation embedded in the fcheap binary, or serve and build the VitePress docs locally.',
  },
  'cli/doctor.md': {
    title: 'fcheap doctor — check runtime health',
    description: 'Check file.cheap paths, configuration, local indexes, and optional runtime dependencies from the terminal.',
  },
  'cli/drop.md': {
    title: 'fcheap drop — delete a local stash',
    description: 'Permanently remove a stash and clean its derived local search metadata with an explicit confirmation.',
  },
  'cli/ecosystem-status.md': {
    title: 'fcheap ecosystem-status — inspect vault usage',
    description: 'Group local stashes by producing tool and estimate storage that cleanup could safely reclaim.',
  },
  'cli/info.md': {
    title: 'fcheap info — inspect stash metadata',
    description: 'Inspect a stash manifest, provenance, tags, compression state, hashes, and complete saved file tree.',
  },
  'cli/list.md': {
    title: 'fcheap list — browse and filter stashes',
    description: 'List local stashes by tag, tool, age, and expiry with stable JSON output for agent workflows.',
  },
  'cli/mcp.md': {
    title: 'fcheap mcp — run the local MCP server',
    description: 'Start the file.cheap stdio MCP server and expose local stash tools, resources, and prompts to AI agents.',
    image: '/og-mcp.png',
  },
  'cli/restore.md': {
    title: 'fcheap restore — recover and verify files',
    description: 'Restore a local stash to a chosen or temporary directory and verify every file against its manifest hash.',
  },
  'cli/save.md': {
    title: 'fcheap save — snapshot files locally',
    description: 'Save a file or folder to a local vault with provenance, tags, secret scanning, hashes, and optional indexing.',
    image: '/og-install.png',
  },
  'cli/search.md': {
    title: 'fcheap search — find files across stashes',
    description: 'Search exact files across local stashes with BM25 keyword, semantic, or hybrid retrieval.',
  },
  'cli/studio.md': {
    title: 'fcheap studio — browse stashes in a TUI',
    description: 'Open the interactive terminal studio to browse, search, restore, compress, diff, and remove local stashes.',
  },
  'cli/sweep.md': {
    title: 'fcheap sweep — clean up expired stashes',
    description: 'Preview or apply TTL-based local stash cleanup with keep tags, filters, and reclaimed-space reporting.',
  },
  'cli/ttl.md': {
    title: 'fcheap ttl — set stash retention',
    description: 'Set, update, or clear the expiry time for a local stash using a duration or calendar date.',
  },
  'cli/vacuum.md': {
    title: 'fcheap vacuum — compact local indexes',
    description: 'Remove orphaned metadata and search entries, then compact file.cheap local indexes safely.',
  },
  'cli/version.md': {
    title: 'fcheap version — inspect build details',
    description: 'Print the installed fcheap version, source commit, and build date for diagnostics and reproducibility.',
  },
  'guide/index.md': {
    title: 'file.cheap documentation',
    description: 'Understand the local artifact vault, choose a first workflow, connect an agent, and find the right command or concept.',
  },
  'guide/getting-started.md': {
    title: 'Getting started with file.cheap',
    description: 'Install the CGO-free fcheap binary, create a local stash, search it, restore it, and configure the vault.',
    image: '/og-install.png',
  },
  'guide/agent-guide.md': {
    title: 'Agent operating guide for file.cheap',
    description: 'Give coding agents a compact, version-matched contract for file.cheap capabilities, safety boundaries, and recommended workflows.',
    image: '/og-mcp.png',
  },
  'guide/core-concepts.md': {
    title: 'file.cheap core concepts',
    description: 'Learn how vaults, stashes, manifests, derived indexes, search modes, restore verification, and retention fit together.',
  },
  'guide/troubleshooting.md': {
    title: 'Troubleshoot file.cheap',
    description: 'Diagnose installation, permissions, search indexing, remote embedding, vecgrep, MCP startup, and restore verification problems.',
  },
  'guide/workflows.md': {
    title: 'Agent artifact workflow examples',
    description: 'Follow complete local workflows for saving, searching, connecting, diffing, restoring, and cleaning agent artifacts.',
  },
  'mcp/overview.md': {
    title: 'MCP file server for Claude Code and Codex',
    description: 'Configure file.cheap as a local stdio MCP server with 15 tools, resources, and investigation prompts.',
    image: '/og-mcp.png',
  },
  'studio/overview.md': {
    title: 'file.cheap Studio — local stash TUI',
    description: 'Browse manifests, preview files, search content, and run stash operations from the file.cheap terminal UI.',
  },
  'integrations/mcp-clients.md': {
    title: 'Connect file.cheap to MCP clients',
    description: 'Add the local file.cheap MCP server to Claude Code, Codex CLI, OpenCode, and any stdio MCP client.',
    image: '/og-mcp.png',
  },
  'integrations/local-artifact-references.md': {
    title: 'Local artifact references for agent tools',
    description: 'Connect Chalupa, Cairntrace, and Glyphrun with versioned local stash references while file.cheap keeps ownership of bytes and restore.',
  },
  'integrations/run-index.md': {
    title: 'Metadata-only run indexes',
    description: 'Index Cairntrace and Glyphrun execution metadata for the private console without opening archives or exposing evidence bytes.',
  },
  'integrations/email-delivery.md': {
    title: 'Email delivery and inbound forwarding',
    description: 'Operate domain-scoped sending and signed inbound forwarding without exposing a public mail relay or private forwarding address.',
  },
  'compare/git-stash-worktree.md': {
    title: 'file.cheap vs Git stash and worktree',
    description: 'Choose the right tool for source changes, parallel branches, and durable agent artifacts with a practical comparison.',
  },
  'compare/cloud-artifact-storage.md': {
    title: 'file.cheap vs cloud artifact storage',
    description: 'Compare a local-first agent artifact vault with generic object storage, cloud drives, and CI artifacts.',
  },
  'learn/index.md': {
    title: 'Learn local-first agent file workflows',
    description: 'Practical guides to MCP file tools, local search, artifact investigations, privacy, and local-first agent stacks.',
  },
  'learn/claude-code-local-file-vault.md': {
    title: 'Give Claude Code a local file vault',
    description: 'Install file.cheap, register its MCP server, and let Claude Code save, search, restore, and inspect local artifacts.',
    image: '/og-mcp.png',
  },
  'learn/local-first-vs-cloud-artifacts.md': {
    title: 'Local-first vs cloud agent artifacts',
    description: 'Understand the privacy, speed, collaboration, recovery, and cost trade-offs for agent artifact storage.',
  },
  'learn/bm25-semantic-hybrid-search.md': {
    title: 'BM25, semantic, and hybrid file search',
    description: 'Learn how keyword, vector, and hybrid retrieval behave on your own files and when to choose each mode.',
  },
  'learn/vidtrace-to-code.md': {
    title: 'From vidtrace evidence to owning code',
    description: 'Turn video-derived OCR and transcripts into a saved, searchable investigation that points back to likely source code.',
  },
  'learn/local-first-agent-stack.md': {
    title: 'Build a local-first agent stack',
    description: 'Combine file.cheap, MCP, Ollama, and optional vecgrep into a private workflow for reusable agent context.',
  },
  'learn/mcp-tools-cheat-sheet.md': {
    title: 'file.cheap MCP tools cheat sheet',
    description: 'Choose among all 15 local MCP tools, resources, and prompts with concise safety and workflow guidance.',
    image: '/og-mcp.png',
  },
}

const softwareApplication = {
  '@type': 'SoftwareApplication',
  '@id': SITE_URL + '/#software',
  name: 'file.cheap',
  applicationCategory: 'DeveloperApplication',
  operatingSystem: 'macOS, Linux',
  description: SITE_DESC,
  url: SITE_URL,
  downloadUrl: 'https://github.com/abdul-hamid-achik/file.cheap/releases/latest',
  license: 'https://github.com/abdul-hamid-achik/file.cheap/blob/main/LICENSE',
  offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
  isAccessibleForFree: true,
  codeRepository: 'https://github.com/abdul-hamid-achik/file.cheap',
  softwareRequirements: 'macOS or Linux',
  author: AUTHOR,
  featureList: [
    'Save and restore file and folder snapshots with hash verification',
    'BM25 keyword, semantic, and hybrid search across stashes',
    'MCP server with 15 tools, resources, and prompts',
    'Versioned local artifact references for cross-tool handoffs',
    'Diff stashes against a live codebase',
    'Trace artifacts to likely owning code with vecgrep',
    'Streaming tar and zstd compression',
    'Save-time secret scanning',
    'Bubbletea Studio terminal interface',
  ],
}

const website = {
  '@type': 'WebSite',
  '@id': SITE_URL + '/#website',
  name: 'file.cheap',
  url: SITE_URL + '/',
  description: SITE_DESC,
  publisher: AUTHOR,
}

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
  cli: '/cli',
  compare: '/compare/git-stash-worktree',
  guide: '/guide',
  integrations: '/integrations/local-artifact-references',
  learn: '/learn',
  mcp: '/mcp/overview',
  studio: '/studio/overview',
}

function routeFromPage(relativePath: string) {
  const route = relativePath
    .replace(/(^|\/)index\.md$/, '$1')
    .replace(/\.md$/, '')
    .replace(/\/$/, '')
  return route ? `/${route}` : '/'
}

function breadcrumbSchema(route: string, title: string) {
  const section = route.split('/').filter(Boolean)[0]
  const sectionRoot = section ? sectionRoots[section] || `/${section}` : '/'
  const items = [
    {
      '@type': 'ListItem',
      position: 1,
      name: 'file.cheap',
      item: SITE_URL,
    },
  ]

  if (section) {
    items.push({
      '@type': 'ListItem',
      position: 2,
      name: sectionLabels[section] || section,
      item: `${SITE_URL}${sectionRoot}`,
    })
  }

  if (route !== sectionRoot) {
    items.push({
      '@type': 'ListItem',
      position: items.length + 1,
      name: title,
      item: `${SITE_URL}${route}`,
    })
  }

  return {
    '@type': 'BreadcrumbList',
    itemListElement: items,
  }
}

const workflowHowTo = {
  '@type': 'HowTo',
  name: 'Investigate an agent artifact with file.cheap',
  description: 'Save and index an artifact, search its files, inspect its provenance, restore verified evidence, and manage retention.',
  step: [
    ['Save and index the artifact', 'Run fcheap save with provenance, tags, and the --index flag.'],
    ['Search its files', 'Run fcheap search and inspect the returned stash ID, path, score, and snippet.'],
    ['Inspect the manifest', 'Run fcheap info to review provenance, hashes, expiry, and secret findings.'],
    ['Restore verified evidence', 'Run fcheap restore to a fresh directory and check the integrity result.'],
    ['Manage retention explicitly', 'Assign a TTL, preview sweep or cleanup, and apply deletion only with deliberate intent.'],
  ].map(([name, text], index) => ({
    '@type': 'HowToStep',
    position: index + 1,
    name,
    text,
  })),
}

const sidebar = {
  '/guide/': [
    {
      text: 'Start here',
      items: [
        { text: 'What file.cheap solves', link: '/guide/' },
        { text: 'Install and create a stash', link: '/guide/getting-started' },
        { text: 'Core concepts', link: '/guide/core-concepts' },
        { text: 'Troubleshooting', link: '/guide/troubleshooting' },
      ],
    },
    {
      text: 'Workflows',
      items: [
        { text: 'Save, find, and restore', link: '/guide/workflows' },
        { text: 'Vidtrace evidence to code', link: '/learn/vidtrace-to-code' },
      ],
    },
    {
      text: 'Agent integrations',
      items: [
        { text: 'Agent operating guide', link: '/guide/agent-guide' },
        { text: 'Share local artifact references', link: '/integrations/local-artifact-references' },
        { text: 'Connect MCP clients', link: '/integrations/mcp-clients' },
        { text: 'Email delivery', link: '/integrations/email-delivery' },
        { text: 'MCP server reference', link: '/mcp/overview' },
      ],
    },
    {
      text: 'Architecture and privacy',
      items: [
        { text: 'Build a local-first agent stack', link: '/learn/local-first-agent-stack' },
        { text: 'Local-first vs cloud artifacts', link: '/learn/local-first-vs-cloud-artifacts' },
      ],
    },
  ],
  '/cli/': [
    {
      text: 'CLI essentials',
      items: [
        { text: 'Overview and global flags', link: '/cli/' },
        { text: 'save', link: '/cli/save' },
        { text: 'artifact-ref', link: '/cli/artifact-ref' },
        { text: 'publish', link: '/cli/publish' },
        { text: 'pull', link: '/cli/pull' },
        { text: 'auth', link: '/cli/auth' },
        { text: 'list', link: '/cli/list' },
        { text: 'info', link: '/cli/info' },
        { text: 'restore', link: '/cli/restore' },
        { text: 'drop', link: '/cli/drop' },
      ],
    },
    {
      text: 'Search and investigate',
      items: [
        { text: 'analyze', link: '/cli/analyze' },
        { text: 'search', link: '/cli/search' },
        { text: 'diff', link: '/cli/diff' },
        { text: 'connect', link: '/cli/connect' },
      ],
    },
    {
      text: 'Storage lifecycle',
      items: [
        { text: 'compress', link: '/cli/compress' },
        { text: 'ttl', link: '/cli/ttl' },
        { text: 'sweep', link: '/cli/sweep' },
        { text: 'cleanup', link: '/cli/cleanup' },
        { text: 'vacuum', link: '/cli/vacuum' },
        { text: 'ecosystem-status', link: '/cli/ecosystem-status' },
      ],
    },
    {
      text: 'Operations',
      items: [
        { text: 'config', link: '/cli/config' },
        { text: 'doctor', link: '/cli/doctor' },
        { text: 'studio', link: '/cli/studio' },
        { text: 'mcp', link: '/cli/mcp' },
        { text: 'docs', link: '/cli/docs' },
        { text: 'completion', link: '/cli/completion' },
        { text: 'version', link: '/cli/version' },
      ],
    },
  ],
  '/mcp/': [
    {
      text: 'MCP',
      items: [
        { text: 'Server overview and schemas', link: '/mcp/overview' },
        { text: 'Share local artifact references', link: '/integrations/local-artifact-references' },
        { text: 'Agent operating guide', link: '/guide/agent-guide' },
        { text: 'Connect clients', link: '/integrations/mcp-clients' },
        { text: 'Tools cheat sheet', link: '/learn/mcp-tools-cheat-sheet' },
        { text: 'mcp CLI command', link: '/cli/mcp' },
      ],
    },
  ],
  '/integrations/': [
    {
      text: 'Agent integrations',
      items: [
        { text: 'Share local artifact references', link: '/integrations/local-artifact-references' },
        { text: 'Metadata-only run indexes', link: '/integrations/run-index' },
        { text: 'Connect MCP clients', link: '/integrations/mcp-clients' },
        { text: 'Email delivery', link: '/integrations/email-delivery' },
        { text: 'Agent operating guide', link: '/guide/agent-guide' },
        { text: 'MCP server reference', link: '/mcp/overview' },
        { text: 'Local-first agent stack', link: '/learn/local-first-agent-stack' },
      ],
    },
  ],
  '/learn/': [
    {
      text: 'Learn',
      items: [
        { text: 'Guide index', link: '/learn/' },
        { text: 'Claude Code local file vault', link: '/learn/claude-code-local-file-vault' },
        { text: 'BM25, semantic, and hybrid search', link: '/learn/bm25-semantic-hybrid-search' },
        { text: 'Vidtrace evidence to code', link: '/learn/vidtrace-to-code' },
        { text: 'Build a local-first agent stack', link: '/learn/local-first-agent-stack' },
        { text: 'MCP tools cheat sheet', link: '/learn/mcp-tools-cheat-sheet' },
        { text: 'Local-first vs cloud artifacts', link: '/learn/local-first-vs-cloud-artifacts' },
      ],
    },
    {
      text: 'Compare',
      items: [
        { text: 'Git stash and worktree', link: '/compare/git-stash-worktree' },
        { text: 'Cloud artifact storage', link: '/compare/cloud-artifact-storage' },
      ],
    },
  ],
  '/compare/': [
    {
      text: 'Compare',
      items: [
        { text: 'Git stash and worktree', link: '/compare/git-stash-worktree' },
        { text: 'Cloud artifact storage', link: '/compare/cloud-artifact-storage' },
        { text: 'Core concepts', link: '/guide/core-concepts' },
      ],
    },
  ],
  '/studio/': [
    {
      text: 'Studio',
      items: [
        { text: 'Studio overview', link: '/studio/overview' },
        { text: 'studio CLI command', link: '/cli/studio' },
        { text: 'Troubleshooting', link: '/guide/troubleshooting' },
      ],
    },
  ],
}

export default defineConfig({
  title: 'file.cheap',
  titleTemplate: ':title | file.cheap',
  description: SITE_DESC,
  lang: 'en-US',
  cleanUrls: true,
  lastUpdated: true,

  head: [
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    [
      'link',
      {
        rel: 'stylesheet',
        href: 'https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&family=Outfit:wght@300;400;500;600;700&display=swap',
      },
    ],
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
    ['meta', { name: 'theme-color', content: '#f4f0e7', media: '(prefers-color-scheme: light)' }],
    ['meta', { name: 'theme-color', content: '#181713', media: '(prefers-color-scheme: dark)' }],
    ['meta', { name: 'color-scheme', content: 'light dark' }],
    ['meta', { name: 'author', content: 'Abdul Hamid Achik' }],
    ['meta', { property: 'og:site_name', content: 'file.cheap' }],
    ['meta', { property: 'og:image:width', content: '1200' }],
    ['meta', { property: 'og:image:height', content: '630' }],
    ['meta', { property: 'og:locale', content: 'en_US' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
  ],

  transformPageData(pageData) {
    pageData.frontmatter.head ??= []

    const meta = PAGE_META[pageData.relativePath]
    if (meta) {
      pageData.title = meta.title
      pageData.description = meta.description
    }

    const isHome = pageData.relativePath === 'index.md'
    if (isHome) pageData.titleTemplate = false

    const route = routeFromPage(pageData.relativePath)
    const canonical = route === '/' ? `${SITE_URL}/` : `${SITE_URL}${route}`
    const title = meta?.title || pageData.title
    const description = meta?.description || pageData.description || SITE_DESC
    const imagePath = meta?.image || '/og.png'
    const image = `${SITE_URL}${imagePath}`
    const imageAlt = meta?.title || SITE_TITLE

    const schemas = isHome
      ? [website, softwareApplication]
      : [
          {
            '@type': 'TechArticle',
            headline: title,
            description,
            url: canonical,
            mainEntityOfPage: { '@type': 'WebPage', '@id': canonical },
            author: AUTHOR,
            isPartOf: { '@type': 'WebSite', name: 'file.cheap', url: SITE_URL },
            ...(pageData.lastUpdated
              ? { dateModified: new Date(pageData.lastUpdated).toISOString() }
              : {}),
          },
          breadcrumbSchema(route, title),
          ...(pageData.relativePath === 'guide/workflows.md' ? [workflowHowTo] : []),
        ]

    pageData.frontmatter.head.push(
      ['meta', { name: 'robots', content: 'index, follow, max-image-preview:large' }],
      ['link', { rel: 'canonical', href: canonical }],
      ['meta', { property: 'og:type', content: isHome ? 'website' : 'article' }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
      ['meta', { property: 'og:url', content: canonical }],
      ['meta', { property: 'og:image', content: image }],
      ['meta', { property: 'og:image:alt', content: imageAlt }],
      ['meta', { name: 'twitter:title', content: title }],
      ['meta', { name: 'twitter:description', content: description }],
      ['meta', { name: 'twitter:image', content: image }],
      ['meta', { name: 'twitter:image:alt', content: imageAlt }],
      [
        'script',
        { type: 'application/ld+json' },
        JSON.stringify({ '@context': 'https://schema.org', '@graph': schemas }),
      ],
    )
  },

  transformHead({ pageData }) {
    if (pageData.isNotFound) {
      return [['meta', { name: 'robots', content: 'noindex, nofollow' }]]
    }
  },

  sitemap: {
    hostname: SITE_URL,
    lastmodDateOnly: false,
    transformItems: (items) =>
      items
        .filter((item) => item.url !== '' && item.url !== '/')
        .map((item) => ({
          ...item,
          url: item.url.replace(/\/$/, ''),
        })),
  },

  themeConfig: {
    siteTitle: 'file.cheap',
    logoLink: {
      link: '/',
      target: '_self',
    },
    sidebar,
    nav: [
      { text: 'Platform', link: '/', target: '_self' },
      { text: 'Get started', link: '/guide/getting-started' },
      { text: 'Workflows', link: '/guide/workflows' },
      { text: 'CLI', link: '/cli/' },
      {
        text: 'MCP',
        items: [
          { text: 'Server overview', link: '/mcp/overview' },
          { text: 'Agent operating guide', link: '/guide/agent-guide' },
          { text: 'Connect clients', link: '/integrations/mcp-clients' },
          { text: 'Local artifact references', link: '/integrations/local-artifact-references' },
        ],
      },
      {
        text: 'Learn',
        items: [
          { text: 'All guides', link: '/learn/' },
          { text: 'Core concepts', link: '/guide/core-concepts' },
          { text: 'Search modes', link: '/learn/bm25-semantic-hybrid-search' },
          { text: 'Local-first agent stack', link: '/learn/local-first-agent-stack' },
          { text: 'Compare approaches', link: '/compare/git-stash-worktree' },
        ],
      },
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/abdul-hamid-achik/file.cheap' },
    ],
    footer: {
      message: 'Local-first. Open source. Released under the MIT License.',
      copyright: 'Copyright © 2026 Abdul Hamid Achik.',
    },
    editLink: {
      pattern: 'https://github.com/abdul-hamid-achik/file.cheap/edit/main/platform/docs/:path',
      text: 'Edit this page on GitHub',
    },
    docFooter: {
      prev: 'Previous',
      next: 'Next',
    },
    outline: {
      level: [2, 3],
    },
    lastUpdated: {
      text: 'Updated',
      formatOptions: {
        dateStyle: 'medium',
      },
    },
    search: {
      provider: 'local',
      options: {
        detailedView: true,
      },
    },
  },
})
