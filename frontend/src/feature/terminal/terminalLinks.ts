const maximumTerminalLinkLength = 2_048
const supportedTerminalLinkPrefix = /^https?:\/\//i

export function sanitizeTerminalLink(value: string): string | undefined {
  if (
    value.length === 0 ||
    value.length > maximumTerminalLinkLength ||
    hasForbiddenTerminalLinkCharacter(value) ||
    !supportedTerminalLinkPrefix.test(value)
  ) {
    return undefined
  }

  try {
    const parsed = new URL(value)
    if (
      (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
      parsed.hostname === '' ||
      parsed.username !== '' ||
      parsed.password !== '' ||
      parsed.href.length > maximumTerminalLinkLength
    ) {
      return undefined
    }
    return parsed.href
  } catch {
    return undefined
  }
}

function hasForbiddenTerminalLinkCharacter(value: string): boolean {
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index)
    if (code <= 0x20 || code === 0x7f || code === 0x5c) return true
  }
  return false
}
