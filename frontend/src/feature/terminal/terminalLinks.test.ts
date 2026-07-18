import { describe, expect, it } from 'vitest'
import { sanitizeTerminalLink } from './terminalLinks'

describe('sanitizeTerminalLink', () => {
  it.each([
    ['https://example.com', 'https://example.com/'],
    ['HTTP://EXAMPLE.COM:80/path?q=one#two', 'http://example.com/path?q=one#two'],
    ['https://[::1]:8443/status', 'https://[::1]:8443/status'],
    ['https://xn--bcher-kva.example/', 'https://xn--bcher-kva.example/'],
  ])('canonicalizes supported web URL %s', (input, expected) => {
    expect(sanitizeTerminalLink(input)).toBe(expected)
  })

  it.each([
    '',
    'example.com/path',
    '//example.com/path',
    'javascript:alert(1)',
    'data:text/html,hello',
    'file:///etc/passwd',
    'ssh://example.com',
    'https:example.com',
    'https:\\example.com',
    'https://user@example.com/private',
    'https://user:secret@example.com/private',
    'https://example.com/line\nbreak',
    ' https://example.com',
    'https://',
    `https://example.com/${'a'.repeat(2_100)}`,
  ])('rejects unsafe or ambiguous URL %s', (input) => {
    expect(sanitizeTerminalLink(input)).toBeUndefined()
  })
})
