export const MAX_PROFILE_ENVIRONMENT_OVERRIDES = 128

const portableEnvironmentName = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/
const managedEnvironmentNames = new Set(['TERM', 'COLORTERM', 'SHHH_SESSION_ID'])

export interface ProfileEnvironmentEntry {
  id: string
  name: string
  value: string
}

export type ProfileEnvironmentResult =
  | { ok: true; environment: Record<string, string> }
  | { ok: false; error: string }

export function profileEnvironmentEntries(environment: Record<string, string>): ProfileEnvironmentEntry[] {
  return Object.entries(environment)
    .sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0)
    .map(([name, value], index) => ({ id: `saved:${index}`, name, value }))
}

export function validateProfileEnvironment(entries: ProfileEnvironmentEntry[]): ProfileEnvironmentResult {
  if (entries.length > MAX_PROFILE_ENVIRONMENT_OVERRIDES) {
    return { ok: false, error: `Profiles support at most ${MAX_PROFILE_ENVIRONMENT_OVERRIDES} environment variables.` }
  }

  const names = new Map<string, string>()
  const pairs: Array<[string, string]> = []
  for (const [index, entry] of entries.entries()) {
    if (entry.name === '') {
      return { ok: false, error: `Environment variable ${index + 1} needs a name.` }
    }
    if (!portableEnvironmentName.test(entry.name)) {
      return {
        ok: false,
        error: `Environment variable "${entry.name}" must start with a letter or underscore and contain only letters, digits, and underscores.`,
      }
    }

    const canonicalName = entry.name.toUpperCase()
    const previous = names.get(canonicalName)
    if (previous) {
      return { ok: false, error: `Environment variables "${previous}" and "${entry.name}" differ only by case.` }
    }
    if (managedEnvironmentNames.has(canonicalName)) {
      return { ok: false, error: `Environment variable "${entry.name}" is managed by shh-h.` }
    }
    if (entry.value.includes('\0')) {
      return { ok: false, error: `Environment variable "${entry.name}" contains a null byte.` }
    }

    names.set(canonicalName, entry.name)
    pairs.push([entry.name, entry.value])
  }

  return { ok: true, environment: Object.fromEntries(pairs) }
}
