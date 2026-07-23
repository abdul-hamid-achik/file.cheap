import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import sharp from 'sharp'

// This is a project-specific generated-output contract, not a general HTML linter.
// It verifies cross-page SEO invariants that generic validators do not know about.
const docsDir = fileURLToPath(new URL('../../', import.meta.url))
const distDir = join(docsDir, '.vitepress', 'dist')
const siteUrl = 'https://file.cheap'
const failures = []

const vercelConfig = JSON.parse(readFileSync(join(docsDir, 'vercel.json'), 'utf8'))
const allRouteHeaders = vercelConfig.headers?.find((rule) => rule.source === '/(.*)')?.headers || []
const responseHeaders = new Map(
  allRouteHeaders.map((header) => [header.key.toLowerCase(), header.value]),
)
for (const name of [
  'content-security-policy',
  'strict-transport-security',
  'x-content-type-options',
  'x-frame-options',
  'referrer-policy',
  'permissions-policy',
]) {
  if (!responseHeaders.has(name)) failures.push(`vercel.json is missing the ${name} header`)
}

const contentSecurityPolicy = responseHeaders.get('content-security-policy') || ''
for (const directive of [
  "default-src 'self'",
  "base-uri 'self'",
  "object-src 'none'",
  "frame-ancestors 'none'",
  "script-src 'self' 'unsafe-inline'",
  "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
  "font-src 'self' data: https://fonts.gstatic.com",
]) {
  if (!contentSecurityPolicy.includes(directive)) {
    failures.push(`vercel.json CSP is missing ${directive}`)
  }
}
if (responseHeaders.get('x-content-type-options') !== 'nosniff') {
  failures.push('vercel.json must set X-Content-Type-Options to nosniff')
}
if (responseHeaders.get('x-frame-options') !== 'DENY') {
  failures.push('vercel.json must set X-Frame-Options to DENY')
}
if (responseHeaders.get('referrer-policy') !== 'strict-origin-when-cross-origin') {
  failures.push('vercel.json must set a strict cross-origin referrer policy')
}
if (!responseHeaders.get('strict-transport-security')?.startsWith('max-age=')) {
  failures.push('vercel.json must set an HSTS max-age')
}
const permissionsPolicy = responseHeaders.get('permissions-policy') || ''
for (const capability of ['camera=()', 'geolocation=()', 'microphone=()', 'payment=()', 'usb=()']) {
  if (!permissionsPolicy.includes(capability)) {
    failures.push(`vercel.json Permissions-Policy is missing ${capability}`)
  }
}

function walk(directory, predicate) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return walk(path, predicate)
    return predicate(path) ? [path] : []
  })
}

function attributes(tag) {
  const values = {}
  for (const match of tag.matchAll(/([:\w-]+)="([^"]*)"/g)) {
    values[match[1]] = match[2]
  }
  return values
}

function metaTags(html, key, value) {
  return [...html.matchAll(/<meta\b[^>]*>/g)]
    .map(([tag]) => attributes(tag))
    .filter((attrs) => attrs[key] === value)
}

function linkTags(html, relValue) {
  return [...html.matchAll(/<link\b[^>]*>/g)]
    .map(([tag]) => attributes(tag))
    .filter((attrs) => (attrs.rel || '').split(/\s+/).includes(relValue))
}

function single(values, label, page) {
  if (values.length !== 1) {
    failures.push(`${page}: expected one ${label}, found ${values.length}`)
    return undefined
  }
  return values[0]
}

function routeFor(relativeHtml) {
  if (relativeHtml === 'index.html') return '/'
  return `/${relativeHtml.replace(/\.html$/, '').replace(/\/index$/, '')}`
}

function canonicalFor(route) {
  return route === '/' ? `${siteUrl}/` : `${siteUrl}${route}`
}

if (!existsSync(distDir)) {
  throw new Error('VitePress output is missing; run bun run docs:build first')
}

const sourceSections = ['cli', 'compare', 'guide', 'integrations', 'learn', 'mcp', 'studio']
const sourceMarkdown = [join(docsDir, 'index.md')]
for (const section of sourceSections) {
  const directory = join(docsDir, section)
  if (existsSync(directory)) {
    sourceMarkdown.push(...walk(directory, (path) => path.endsWith('.md')))
  }
}

for (const source of sourceMarkdown) {
  const output = join(distDir, relative(docsDir, source).replace(/\.md$/, '.html'))
  if (!existsSync(output)) failures.push(`missing rendered page for ${relative(docsDir, source)}`)
}

const htmlFiles = walk(distDir, (path) => path.endsWith('.html')).sort()
const expectedHtmlCount = sourceMarkdown.length + 1
if (htmlFiles.length !== expectedHtmlCount) {
  failures.push(`expected ${expectedHtmlCount} HTML files including 404, found ${htmlFiles.length}`)
}

const seenTitles = new Map()
const seenDescriptions = new Map()
const seenCanonicals = new Map()
const expectedSitemapUrls = new Set()

for (const file of htmlFiles) {
  const page = relative(distDir, file).replaceAll('\\', '/')
  const html = readFileSync(file, 'utf8')
  const isNotFound = page === '404.html'

  const titleMatches = [...html.matchAll(/<title>([^<]+)<\/title>/g)].map((match) => match[1])
  const title = single(titleMatches, 'title', page)
  const descriptions = metaTags(html, 'name', 'description')
  const description = single(descriptions, 'meta description', page)?.content
  const robots = single(metaTags(html, 'name', 'robots'), 'robots directive', page)?.content || ''

  if (metaTags(html, 'charset', 'utf-8').length !== 1) {
    failures.push(`${page}: expected one UTF-8 charset declaration`)
  }
  if (metaTags(html, 'name', 'viewport').length !== 1) {
    failures.push(`${page}: expected one viewport declaration`)
  }
  if (html.includes('as="font"')) failures.push(`${page}: unexpected bundled font preload`)

  if (isNotFound) {
    if (!robots.includes('noindex')) failures.push('404.html: missing noindex directive')
    if (linkTags(html, 'canonical').length) failures.push('404.html: must not have a canonical URL')
    if (html.includes('application/ld+json')) failures.push('404.html: must not have structured data')
    continue
  }

  const route = routeFor(page)
  const expectedCanonical = canonicalFor(route)
  if (route !== '/') expectedSitemapUrls.add(expectedCanonical)

  if (!robots.includes('index') || robots.includes('noindex')) {
    failures.push(`${page}: page is not explicitly indexable`)
  }

  const canonical = single(linkTags(html, 'canonical'), 'canonical URL', page)?.href
  const ogUrl = single(metaTags(html, 'property', 'og:url'), 'og:url', page)?.content
  const ogTitle = single(metaTags(html, 'property', 'og:title'), 'og:title', page)?.content
  const ogDescription = single(metaTags(html, 'property', 'og:description'), 'og:description', page)?.content
  const ogImage = single(metaTags(html, 'property', 'og:image'), 'og:image', page)?.content
  single(metaTags(html, 'name', 'twitter:title'), 'twitter:title', page)
  single(metaTags(html, 'name', 'twitter:description'), 'twitter:description', page)
  single(metaTags(html, 'name', 'twitter:image'), 'twitter:image', page)

  if (canonical !== expectedCanonical) {
    failures.push(`${page}: canonical ${canonical} does not match ${expectedCanonical}`)
  }
  if (ogUrl !== expectedCanonical) failures.push(`${page}: og:url does not match canonical`)
  if (ogTitle && title && title !== ogTitle && title !== `${ogTitle} | file.cheap`) {
    failures.push(`${page}: HTML title and og:title disagree`)
  }
  if (ogDescription !== description) failures.push(`${page}: description and og:description disagree`)

  if (title && title.length > 60) failures.push(`${page}: title is ${title.length} characters`)
  if (description && description.length > 160) {
    failures.push(`${page}: description is ${description.length} characters`)
  }

  for (const [map, value, label] of [
    [seenTitles, title, 'title'],
    [seenDescriptions, description, 'description'],
    [seenCanonicals, canonical, 'canonical'],
  ]) {
    if (!value) continue
    if (map.has(value)) failures.push(`${page}: duplicate ${label} also used by ${map.get(value)}`)
    else map.set(value, page)
  }

  if (!ogImage?.startsWith(`${siteUrl}/`)) {
    failures.push(`${page}: og:image is not an absolute file.cheap URL`)
  } else {
    const imagePath = join(distDir, new URL(ogImage).pathname)
    if (!existsSync(imagePath)) failures.push(`${page}: referenced OG image is missing`)
  }

  const schemaMatches = [...html.matchAll(/<script type="application\/ld\+json">([^<]+)<\/script>/g)]
  const schemaText = single(schemaMatches.map((match) => match[1]), 'JSON-LD graph', page)
  if (schemaText) {
    try {
      const schema = JSON.parse(schemaText)
      const types = new Set((schema['@graph'] || []).map((entry) => entry['@type']))
      if (route === '/') {
        if (!types.has('SoftwareApplication') || !types.has('WebSite') || types.has('TechArticle')) {
          failures.push(`${page}: home schema must describe the WebSite and SoftwareApplication`)
        }
      } else {
        if (!types.has('TechArticle')) failures.push(`${page}: missing TechArticle schema`)
        if (!types.has('BreadcrumbList')) failures.push(`${page}: missing BreadcrumbList schema`)
        if (route === '/guide/workflows' && !types.has('HowTo')) {
          failures.push(`${page}: missing HowTo schema`)
        }
      }
    } catch (error) {
      failures.push(`${page}: invalid JSON-LD (${error.message})`)
    }
  }

  if (route !== '/' && !html.includes('class="fc-breadcrumbs"')) {
    failures.push(`${page}: visible breadcrumbs are missing`)
  }
}

function internalOutput(pathname) {
  let cleanPath
  try {
    cleanPath = decodeURIComponent(pathname)
  } catch {
    return undefined
  }

  if (cleanPath === '/') return join(distDir, 'index.html')

  const clean = cleanPath.replace(/^\/+|\/+$/g, '')
  const candidates = clean.endsWith('.html')
    ? [join(distDir, clean)]
    : [join(distDir, `${clean}.html`), join(distDir, clean, 'index.html'), join(distDir, clean)]

  return candidates.find((candidate) => existsSync(candidate))
}

// Validate the rendered graph rather than only source Markdown. This catches
// broken navigation, generated sidebar links, missing public assets, and stale
// fragments after heading edits.
for (const file of htmlFiles) {
  const page = relative(distDir, file).replaceAll('\\', '/')
  if (page === '404.html') continue

  const html = readFileSync(file, 'utf8')
  const route = routeFor(page)
  const isIndex = page === 'index.html' || page.endsWith('/index.html')
  const canonical = canonicalFor(route)
  const base = isIndex && !canonical.endsWith('/') ? `${canonical}/` : canonical

  for (const [, tag] of html.matchAll(/(<a\b[^>]*>)/g)) {
    const href = attributes(tag).href?.replaceAll('&amp;', '&')
    if (!href) continue

    let targetUrl
    try {
      targetUrl = new URL(href, base)
    } catch {
      failures.push(`${page}: invalid link ${href}`)
      continue
    }

    if (targetUrl.origin !== siteUrl) continue

    const output = internalOutput(targetUrl.pathname)
    if (!output) {
      failures.push(`${page}: internal link target is missing (${href})`)
      continue
    }

    if (!targetUrl.hash || !output.endsWith('.html')) continue

    let fragment
    try {
      fragment = decodeURIComponent(targetUrl.hash.slice(1))
    } catch {
      failures.push(`${page}: invalid link fragment (${href})`)
      continue
    }

    const targetHtml = readFileSync(output, 'utf8')
    const ids = new Set(
      [...targetHtml.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]),
    )
    if (!ids.has(fragment)) failures.push(`${page}: link fragment is missing (${href})`)
  }
}

const sitemap = readFileSync(join(distDir, 'sitemap.xml'), 'utf8')
const sitemapUrls = new Set([...sitemap.matchAll(/<loc>([^<]+)<\/loc>/g)].map((match) => match[1]))
if (sitemapUrls.has(`${siteUrl}/`)) {
  failures.push('docs sitemap must not include the platform root')
}
for (const url of expectedSitemapUrls) {
  if (!sitemapUrls.has(url)) failures.push(`sitemap is missing ${url}`)
}
for (const url of sitemapUrls) {
  if (!expectedSitemapUrls.has(url)) failures.push(`sitemap has unexpected URL ${url}`)
}

const robotsText = readFileSync(join(distDir, 'robots.txt'), 'utf8')
if (!robotsText.includes('Sitemap: https://file.cheap/docs-sitemap.xml')) {
  failures.push('robots.txt does not advertise the canonical docs sitemap')
}

const securityText = readFileSync(join(distDir, '.well-known', 'security.txt'), 'utf8')
for (const field of ['Contact:', 'Expires:', 'Canonical:', 'Policy:']) {
  if (!securityText.includes(field)) failures.push(`security.txt is missing ${field}`)
}

for (const imageName of ['og.png', 'og-install.png', 'og-mcp.png']) {
  const metadata = await sharp(join(distDir, imageName)).metadata()
  if (metadata.width !== 1200 || metadata.height !== 630 || metadata.format !== 'png') {
    failures.push(`${imageName}: expected a 1200x630 PNG`)
  }
}

if (failures.length) {
  console.error(`SEO verification failed with ${failures.length} issue(s):`)
  for (const failure of failures) console.error(`- ${failure}`)
  process.exitCode = 1
} else {
  console.log(`SEO verification passed for ${sourceMarkdown.length} pages plus 404.`)
}
