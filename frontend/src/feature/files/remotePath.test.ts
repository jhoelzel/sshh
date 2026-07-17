import { describe, expect, it } from 'vitest'
import { canonicalRemotePath, joinRemotePath, parentRemotePath } from './remotePath'

describe('remote path helpers', () => {
  it('canonicalizes absolute POSIX paths', () => {
    expect(canonicalRemotePath('/srv//app/../logs/.')).toBe('/srv/logs')
    expect(canonicalRemotePath('/')).toBe('/')
    expect(canonicalRemotePath('/srv/logs ')).toBe('/srv/logs ')
  })

  it('joins and navigates without escaping the root', () => {
    expect(joinRemotePath('/srv/app', '../logs')).toBe('/srv/logs')
    expect(parentRemotePath('/srv')).toBe('/')
    expect(parentRemotePath('/')).toBe('/')
  })

  it('rejects relative and control-character paths', () => {
    expect(() => canonicalRemotePath('srv/app')).toThrow('absolute')
    expect(() => canonicalRemotePath('/srv\napp')).toThrow('control character')
  })
})
