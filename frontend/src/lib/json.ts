const JSON_TYPES = new Set(['json', 'jsonb'])

/** The value indented over lines when it is a JSON object or array, else null. */
export function prettyJson(v: string | null, type = ''): string | null {
  if (v == null) return null
  const s = v.trim()
  if (!JSON_TYPES.has(type.toLowerCase()) && !/^[[{]/.test(s)) return null
  try {
    const parsed = JSON.parse(s)
    return parsed !== null && typeof parsed === 'object' ? JSON.stringify(parsed, null, 2) : null
  } catch {
    return null
  }
}
