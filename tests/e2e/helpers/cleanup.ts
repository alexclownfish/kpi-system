import { mkdir, writeFile } from "node:fs/promises"
import path from "node:path"
import type { TemporaryRecord } from "./acceptance-context"
import { safeError } from "./redaction"

type DeleteRecord = (record: TemporaryRecord) => Promise<void>

export class CleanupRegistry {
  readonly records: TemporaryRecord[] = []

  register(record: Omit<TemporaryRecord, "cleanupStatus">): void {
    this.records.push({ ...record, cleanupStatus: "registered" })
  }

  async cleanup(deleteRecord: DeleteRecord): Promise<void> {
    const failures: TemporaryRecord[] = []
    for (const record of [...this.records].sort((a, b) => a.cleanupOrder - b.cleanupOrder)) {
      try {
        await deleteRecord(record)
        record.cleanupStatus = "deleted"
      } catch (error) {
        record.cleanupStatus = "failed"
        record.cleanupError = safeError(error)
        failures.push(record)
      }
    }
    await this.writeStatus(failures.length ? "failed" : "complete")
    if (failures.length) {
      throw new Error(`Cleanup failed for ${failures.map(item => `${item.resource} id=${item.id}`).join(", ")}`)
    }
  }

  private async writeStatus(status: "complete" | "failed"): Promise<void> {
    const artifactDir = path.resolve(".artifacts")
    await mkdir(artifactDir, { recursive: true })
    await writeFile(
      path.join(artifactDir, "cleanup-status.json"),
      JSON.stringify({ status, records: this.records }, null, 2),
      "utf8",
    )
  }
}
