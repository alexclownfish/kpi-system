const JWT_PATTERN = /\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/g
const BCRYPT_PATTERN = /\$2[aby]\$\d{2}\$[./A-Za-z0-9]{53}/g
const SECRET_KEY_PATTERN = /(password|authorization|cookie|set-cookie|token|secret)/i

export function redactText(value: string): string {
  let redacted = value
    .replace(JWT_PATTERN, "[REDACTED_JWT]")
    .replace(BCRYPT_PATTERN, "[REDACTED_HASH]")
    .replace(/(Bearer\s+)[^\s,;]+/gi, "$1[REDACTED]")
    .replace(/((?:password|authorization|cookie|set-cookie|token|secret)\s*[=:]\s*)[^\s,;&]+/gi, "$1[REDACTED]")
  for (const secret of [process.env.KPI_TEST_HR_PASSWORD, process.env.KPI_TEST_TEMP_PASSWORD]) {
    if (secret) redacted = redacted.split(secret).join("[REDACTED]")
  }
  return redacted
}

export function redact<T>(value: T): T {
  if (typeof value === "string") return redactText(value) as T
  if (Array.isArray(value)) return value.map(item => redact(item)) as T
  if (value && typeof value === "object") {
    const result: Record<string, unknown> = {}
    for (const [key, item] of Object.entries(value)) {
      result[key] = SECRET_KEY_PATTERN.test(key) ? "[REDACTED]" : redact(item)
    }
    return result as T
  }
  return value
}

export function safeError(error: unknown): string {
  if (error instanceof Error) return redactText(error.message)
  if (error && typeof error === "object" && "message" in error && typeof error.message === "string") {
    return redactText(error.message)
  }
  try {
    return redactText(JSON.stringify(redact(error)))
  } catch {
    return "Unknown acceptance error"
  }
}
