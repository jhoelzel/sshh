export function canonicalRemotePath(value: string): string {
  if (!value.startsWith('/')) throw new Error('Remote path must be absolute')
  if ([...value].some((character) => {
    const codePoint = character.codePointAt(0) ?? 0
    return codePoint < 0x20 || codePoint === 0x7f
  })) {
    throw new Error('Remote path contains a control character')
  }
  const parts: string[] = []
  for (const part of value.split('/')) {
    if (!part || part === '.') continue
    if (part === '..') {
      parts.pop()
    } else {
      parts.push(part)
    }
  }
  return `/${parts.join('/')}`
}

export function joinRemotePath(directory: string, name: string): string {
  const trimmedName = name.trim()
  if (!trimmedName) throw new Error('Remote item name is required')
  return canonicalRemotePath(`${directory}/${trimmedName}`)
}

export function parentRemotePath(value: string): string {
  const canonical = canonicalRemotePath(value)
  const parts = canonical.split('/').filter(Boolean)
  parts.pop()
  return `/${parts.join('/')}`
}
