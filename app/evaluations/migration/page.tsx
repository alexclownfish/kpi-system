"use client"

import { useState } from "react"
import axios from "axios"
import { AlertTriangle, Play, ShieldCheck } from "lucide-react"
import { assessmentMigrationApi } from "@/lib/api"
import { useAuth } from "@/lib/auth-context"
import { useAppContext } from "@/lib/app-context"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

function message(error: unknown) {
  if (axios.isAxiosError(error)) {
    const payload = error.response?.data as { error?: string | { message?: string } } | undefined
    if (typeof payload?.error === "string") return payload.error
    if (payload?.error && typeof payload.error === "object" && payload.error.message) return payload.error.message
  }
  return error instanceof Error ? error.message : "操作失败，请稍后重试"
}

export default function EvaluationMigrationPage() {
  const { hasPermission } = useAuth()
  const { Alert, Confirm } = useAppContext()
  const [ids, setIds] = useState("")
  const [batch, setBatch] = useState<{ batch_id: string; eligible_evaluation_ids: number[]; manual_review: Array<Record<string, unknown>>; planned_task_count: number } | null>(null)
  const [busy, setBusy] = useState(false)

  if (!hasPermission("system:edit")) return <div className="p-6">您没有迁移权限。</div>

  const dryRun = async () => {
    try {
      setBusy(true)
      const evaluationIds = ids.split(/[,\s]+/).map(value => Number(value)).filter(value => Number.isInteger(value) && value > 0)
      const response = await assessmentMigrationApi.dryRun({ evaluation_ids: evaluationIds, target_version: "weighted-v2" })
      setBatch(response.data)
      await Alert("预演完成", `可迁移 ${response.data.eligible_evaluation_ids.length} 条，计划生成 ${response.data.planned_task_count} 个评分任务。`)
    } catch (error) { await Alert("预演失败", message(error)) } finally { setBusy(false) }
  }

  const apply = async () => {
    if (!batch || !(await Confirm("确认执行迁移", "只会处理预演通过且来源未变化的考核；旧评分记录不会删除。"))) return
    try {
      setBusy(true)
      const response = await assessmentMigrationApi.apply(batch.batch_id)
      await Alert("迁移完成", `已生成 ${response.data.created_task_count} 个评分任务。`)
    } catch (error) { await Alert("迁移失败", message(error)) } finally { setBusy(false) }
  }

  return <div className="space-y-6 p-4 md:p-6">
    <div><h1 className="text-2xl font-bold">考核迁移</h1><p className="text-sm text-muted-foreground">先预演，再执行。系统不会删除旧评分、邀请或总分。</p></div>
    <Card><CardHeader><CardTitle className="flex items-center gap-2"><ShieldCheck className="h-5 w-5" />安全迁移预演</CardTitle></CardHeader><CardContent className="space-y-4">
      <div><Label>考核 ID（可选，逗号或空格分隔）</Label><Input value={ids} onChange={event => setIds(event.target.value)} placeholder="留空表示当前数据范围内全部考核" /></div>
      <div className="flex gap-2"><Button disabled={busy} onClick={dryRun}><Play className="mr-2 h-4 w-4" />运行预演</Button>{batch && <Button variant="outline" disabled={busy || batch.eligible_evaluation_ids.length === 0} onClick={apply}>执行已审核批次</Button>}</div>
      {batch && <div className="rounded-md border p-4 text-sm"><div>批次：{batch.batch_id}</div><div>可迁移：{batch.eligible_evaluation_ids.join(", ") || "无"}</div><div>计划任务：{batch.planned_task_count}</div>{batch.manual_review.length > 0 && <div className="mt-3 rounded bg-amber-50 p-3 text-amber-900"><div className="flex items-center gap-2 font-medium"><AlertTriangle className="h-4 w-4" />人工复核项</div><ul className="mt-1 list-disc pl-5">{batch.manual_review.map((item, index) => <li key={index}>考核 #{String(item.evaluation_id)}：{String(item.reason || item.code)}</li>)}</ul></div>}</div>}
    </CardContent></Card>
  </div>
}
