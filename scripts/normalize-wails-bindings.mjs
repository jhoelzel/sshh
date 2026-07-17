import { readdir, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const generatedRoot = fileURLToPath(new URL('../frontend/wailsjs/go/', import.meta.url))

async function generatedFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true })
  const files = await Promise.all(entries.map(async (entry) => {
    const path = join(directory, entry.name)
    return entry.isDirectory() ? generatedFiles(path) : [path]
  }))
  return files.flat().filter((path) => path.endsWith('.js') || path.endsWith('.ts'))
}

for (const path of await generatedFiles(generatedRoot)) {
  const source = await readFile(path, 'utf8')
  const normalized = source
    .replace(/\r\n/g, '\n')
    .replace(/[ \t]+$/gm, '')
    .replace(/\n*$/, '\n')
  if (normalized !== source) {
    await writeFile(path, normalized, 'utf8')
  }
}
