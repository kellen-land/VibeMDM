export type ClassValue = string | number | bigint | null | false | undefined | ClassValue[] | Record<string, boolean | null | undefined>

export function cn(...args: ClassValue[]): string {
  const out: string[] = []
  for (const a of args) {
    if (!a) continue
    if (typeof a === "string" || typeof a === "number" || typeof a === "bigint") out.push(String(a))
    else if (Array.isArray(a)) {
      const s = cn(...a)
      if (s) out.push(s)
    } else {
      for (const [k, v] of Object.entries(a)) if (v) out.push(k)
    }
  }
  return out.join(' ')
}
