import { readdir, readFile, stat } from "node:fs/promises"
import path from "node:path"

const roots = process.argv.slice(2)
const exactSecrets = [process.env.KPI_TEST_HR_PASSWORD, process.env.KPI_TEST_TEMP_PASSWORD].filter(Boolean)
const textExtensions = new Set([".html", ".json", ".xml", ".txt", ".md", ".zip"])
const jwtPattern = /\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/
const bcryptPattern = /\$2[aby]\$\d{2}\$[./A-Za-z0-9]{53}/
const bearerPattern = /Bearer\s+(?!\[REDACTED\])[^\s"'<]+/i
const cookieValuePattern = /(?:set-cookie|cookie)\s*[=:]\s*(?!\[REDACTED\])[^\s"'<]+/i

async function filesUnder(root) {
  try {
    const info = await stat(root)
    if (info.isFile()) return [root]
    const entries = await readdir(root, { withFileTypes: true })
    const nested = await Promise.all(entries.map(entry => filesUnder(path.join(root, entry.name))))
    return nested.flat()
  } catch {
    return []
  }
}

const files = (await Promise.all(roots.map(filesUnder))).flat()
const leaks = []
for (const file of files) {
  if (!textExtensions.has(path.extname(file).toLowerCase())) continue
  const content = await readFile(file)
  const text = content.toString("latin1")
  const reasons = []
  for (const secret of exactSecrets) {
    if (secret && text.includes(secret)) reasons.push("exact test secret")
  }
  if (jwtPattern.test(text)) reasons.push("complete JWT")
  if (bcryptPattern.test(text)) reasons.push("bcrypt hash")
  if (bearerPattern.test(text)) reasons.push("Bearer credential")
  if (cookieValuePattern.test(text)) reasons.push("Cookie credential")
  if (reasons.length) leaks.push({ file, reasons: [...new Set(reasons)] })
}

if (leaks.length) {
  for (const leak of leaks) console.error(`[SECRET LEAK] ${leak.file}: ${leak.reasons.join(", ")}`)
  process.exit(1)
}
console.log(`[PASS] artifact secret scan (${files.length} files inspected)`)
