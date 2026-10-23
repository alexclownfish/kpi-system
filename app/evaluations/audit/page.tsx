"use client"

import { useState } from "react"
import { FileSearch, Loader2, Play } from "lucide-react"
import { assessmentAuditApi, getApiErrorMessage, type AssessmentAuditPreview, type AssessmentAuditFinding } from "@/lib/api"
import { useAuth } from "@/lib/auth-context"
import { useAppContext } from "@/lib/app-context"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

const severityLabels: Record<AssessmentAuditFinding["severity"], string> = {
  info: "通过",
  warning: "提示",
  error: "错误",
  manual_review: "需人工处理",
}

function severityVariant(severity: AssessmentAuditFinding["severity"]): "default" | "secondary" | "destructive" | "outline" {
  if (severity === "error" || severity === "manual_review") return "destructive"
  if (severity === "warning") return "secondary"
  return "outline"
}

export default function EvaluationAuditPage() {
  const { hasPermission } = useAuth()
  const { Alert, Confirm } = useAppContext()
  const [period, setPeriod] = useState("")
  const [includeCompleted, setIncludeCompleted] = useState(false)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<AssessmentAuditPreview | null>(null)
  const [cleaningOrphans, setCleaningOrphans] = useState(false)

  const canAudit = hasPermission("audit:view") || hasPermission("system:edit")

  const runAudit = async () => {
    try {
      setLoading(true)
      const response = await assessmentAuditApi.preview({
        period: period.trim() || undefined,
        include_completed: includeCompleted,
      })
      setResult(response.data)
    } catch (error) {
      await Alert("审计失败", getApiErrorMessage(error, "无法完成考核数据审计，请检查权限和服务状态。"))
    } finally {
      setLoading(false)
    }
  }

  const cleanupOrphans = async (finding: AssessmentAuditFinding) => {
    const details = finding.details as { sample_ids?: number[] } | undefined
    const ids = details?.sample_ids || []
    if (ids.length === 0) {
      await Alert("无法清理", "该发现没有可清理的任务ID，请先备份数据库并人工核查。")
      return
    }
    const confirmed = await Confirm("清理孤立评分任务", `将删除 ${ids.length} 条找不到所属考核的评分任务。建议先完成数据库备份，确认继续？`)
    if (!confirmed) return
    setCleaningOrphans(true)
    try {
      const response = await assessmentAuditApi.cleanupOrphanScoringTasks(ids)
      await Alert("清理完成", `已清理 ${response.data.deleted} 条孤立评分任务。`)
      await runAudit()
    } catch (error) {
      await Alert("清理失败", getApiErrorMessage(error, "孤立评分任务清理失败"))
    } finally {
      setCleaningOrphans(false)
    }
  }

  if (!canAudit) {
    return <div className="p-6 text-sm text-muted-foreground">当前账号没有考核审计权限。</div>
  }

  return (
    <div className="space-y-6 p-4 md:p-6">
      <div>
        <div className="flex items-center gap-2">
          <FileSearch className="h-6 w-6" />
          <h1 className="text-2xl font-bold">考核数据审计</h1>
        </div>
        <p className="mt-2 text-sm text-muted-foreground">只读检查模板快照、评分记录、评分任务和状态，不会修改现有考核数据。</p>
      </div>

      <Card>
        <CardHeader><CardTitle>审计范围</CardTitle></CardHeader>
        <CardContent className="flex flex-col gap-4 sm:flex-row sm:items-end">
          <div className="space-y-2">
            <Label htmlFor="audit-period">考核周期（可选）</Label>
            <Input id="audit-period" value={period} onChange={event => setPeriod(event.target.value)} placeholder="例如：2026-Q3" className="sm:w-56" />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={includeCompleted} onChange={event => setIncludeCompleted(event.target.checked)} />
            包含已完成考核
          </label>
          <Button onClick={runAudit} disabled={loading}>
            {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Play className="mr-2 h-4 w-4" />}
            {loading ? "审计中…" : "运行只读审计"}
          </Button>
        </CardContent>
      </Card>

      {result && (
        <>
          <div className="grid gap-4 sm:grid-cols-4">
            <SummaryCard title="扫描考核" value={result.summary.evaluations_scanned} />
            <SummaryCard title="可进入演练" value={result.summary.eligible} />
            <SummaryCard title="需人工处理" value={result.summary.manual_review} />
            <SummaryCard title="错误" value={result.summary.errors} />
          </div>
          <Card>
            <CardHeader><CardTitle>审计发现</CardTitle></CardHeader>
            <CardContent>
              {result.findings.length === 0 ? <div className="py-8 text-center text-sm text-muted-foreground">当前筛选范围没有发现记录。</div> : (
                <Table>
                  <TableHeader><TableRow><TableHead>考核</TableHead><TableHead>类别</TableHead><TableHead>状态</TableHead><TableHead>代码</TableHead><TableHead>说明</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
                  <TableBody>
                    {result.findings.map((finding, index) => (
                      <TableRow key={`${finding.evaluation_id ?? "global"}-${finding.code}-${index}`}>
                        <TableCell>{finding.evaluation_id ?? "-"}</TableCell>
                        <TableCell>{finding.category}</TableCell>
                        <TableCell><Badge variant={severityVariant(finding.severity)}>{severityLabels[finding.severity]}</Badge></TableCell>
                        <TableCell className="font-mono text-xs">{finding.code}</TableCell>
                        <TableCell>
                          <div>{finding.message}</div>
                          {finding.details != null && <pre className="mt-1 max-w-xl whitespace-pre-wrap text-xs text-muted-foreground">{JSON.stringify(finding.details) || ""}</pre>}
                        </TableCell>
                        <TableCell>
                          {hasPermission("system:edit") && finding.code === "ORPHAN_SCORING_TASKS" && (
                            <Button size="sm" variant="outline" disabled={cleaningOrphans} onClick={() => cleanupOrphans(finding)}>清理样本</Button>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}

function SummaryCard({ title, value }: { title: string; value: number }) {
  return <Card><CardContent className="pt-6"><div className="text-sm text-muted-foreground">{title}</div><div className="mt-2 text-2xl font-semibold">{value}</div></CardContent></Card>
}
