import { readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import sharp from 'sharp'

const publicDir = new URL('../../public/', import.meta.url)
const sources = readdirSync(publicDir)
  .filter((name) => /^og(?:-[a-z0-9-]+)?\.svg$/.test(name))
  .sort()

for (const source of sources) {
  const target = source.replace(/\.svg$/, '.png')
  const input = fileURLToPath(new URL(source, publicDir))
  const output = fileURLToPath(new URL(target, publicDir))
  await sharp(input, { density: 144 })
    .resize(1200, 630, { fit: 'fill' })
    .png()
    .toFile(output)
  console.log(`${target} written`)
}
