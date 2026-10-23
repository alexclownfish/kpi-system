"use client"

import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { Loader2, Save } from "lucide-react"
import { evaluationApi, scoreApi, type CanonicalEvaluationResult, type EvaluationScoringTaskList, getApiErrorMessage, getApiErrorRequestId } from "@/lib/api"

type Props = {
  evaluationId: number
  onUpdated?: () => void
}

const scorerLabels: Record<string, string> = {
  PM: "项目经理",
  DEPARTMENT_MANAGER: "部门负责人",
  PRODUCT_MANAGER: "产品经理",
  CTO: "CTO",
  DIRECT_MANAGER: "直属负责人",
}

const workflowLabels: Record<string, string> = {
  draft: "草稿",
  self_evaluating: "员工自评",
  scoring: "按模板评分",
  awaiting_review: "等待复核",
  awaiting_employee_confirmation: "等待员工确认",
  completed: "已完成",
  rejected: "已退回",
  cancelled: "已取消",
}

const taskLabels: Record<string, string> = {
  pending: "待评分", in_progress: "评分中", submitted: "已提交",
  rejected: "已退回", not_applicable: "不适用", locked: "已锁定",
}

export function EvaluationScoringTaskList({ evaluationId, onUpdated }: Props) {
  const [data, setData] = useState<EvaluationScoringTaskList | null>(null)
  const [result, setResult] = useState<CanonicalEvaluationResult | null>(null)
  const [loading, setLoading] = useState(true)
  const [busyId, setBusyId] = useState<number | null>(null)
  const [advancing, setAdvancing] = useState(false)
  const [message, setMessage] = useState<string | null>(null)

  const completionPercent = data?.completion.required
    ? Math.round((data.completion.submitted / data.completion.required) * 100)
    : 100

  const refresh = async () => {
    try {
      setLoading(true)
      setMessage(null)
      const response = await evaluationApi.getScoringTasks(evaluationId)
      setData(response.data)
      try {
        const resultResponse = await evaluationApi.getCanonicalResult(evaluationId)
        setResult(resultResponse.data)
      } catch {
        setResult(null)
      }
    } catch (error) {
      setMessage(getApiErrorMessage(error, "当前考核没有可读取的新版评分任务"))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [evaluationId])

  const updateTask = async (taskId: number, score: string, comment: string, version: number, submit: boolean, notApplicable: boolean) => {
    const task = data?.tasks.find(item => item.id === taskId)
    if (!task) return
    const parsed = score.trim() === "" ? null : Number(score)
    if (parsed !== null && Number.isNaN(parsed)) {
      setMessage("评分必须是数字")
      return
    }
    try {
      setBusyId(taskId)
      setMessage(null)
      if (submit) {
        await scoreApi.submitTask(taskId, { score: notApplicable ? null : parsed, comment, not_applicable: notApplicable, version })
      } else {
        await scoreApi.saveTask(taskId, { score: notApplicable ? null : parsed, comment, not_applicable: notApplicable, version })
      }
      await refresh()
      onUpdated?.()
    } catch (error) {
      const requestId = getApiErrorRequestId(error)
      setMessage(`${getApiErrorMessage(error, "评分任务保存失败，请刷新后重试")}${requestId ? `（请求 ID：${requestId}）` : ""}`)
    } finally {
      setBusyId(null)
    }
  }

  const advanceWorkflow = async () => {
    if (!data) return
    try {
      setAdvancing(true)
      setMessage(null)
      await evaluationApi.advanceWorkflow(evaluationId, data.workflow_state)
      await refresh()
      onUpdated?.()
    } catch (error) {
      const requestId = getApiErrorRequestId(error)
      setMessage(`${getApiErrorMessage(error, "当前流程暂时不能推进，请先完成阻塞任务")}${requestId ? `（请求 ID：${requestId}）` : ""}`)
    } finally {
      setAdvancing(false)
    }
  }

  if (loading && !data) {
    return <div className="rounded-md border p-4 text-sm text-muted-foreground">正在读取新版评分任务...</div>
  }
  if (!data) return null

  return (
    <Card className="border-blue-200 dark:border-blue-800">
      <CardHeader className="pb-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle className="text-base">按模板分配的评分任务</CardTitle>
          <Badge variant="outline">状态：{workflowLabels[data.workflow_state] || data.workflow_state}</Badge>
        </div>
        <p className="text-sm text-muted-foreground">
          必填任务 {data.completion.submitted}/{data.completion.required}，待完成 {data.completion.blocking}
        </p>
        <div className="h-2 w-full overflow-hidden rounded-full bg-muted" aria-label="必填评分任务完成度">
          <div className="h-full rounded-full bg-blue-600 transition-all" style={{ width: `${completionPercent}%` }} />
        </div>
        {!(["awaiting_employee_confirmation", "completed", "cancelled"].includes(data.workflow_state)) && (
          <Button size="sm" variant="outline" onClick={advanceWorkflow} disabled={advancing}>
            {advancing && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}推进流程
          </Button>
        )}
      </CardHeader>
      <CardContent className="space-y-3">
        {message && (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
            <span>{message}</span>
            <Button type="button" size="sm" variant="outline" onClick={refresh} disabled={loading}>重新读取</Button>
          </div>
        )}
        {!!data.blocking_tasks?.length && (
          <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
            <div className="font-medium">当前还不能推进流程：</div>
            <ul className="mt-1 list-disc pl-5">
              {data.blocking_tasks.slice(0, 8).map((blocking, index) => (
                <li key={`${String(blocking.task_id || blocking.item_id || "blocking")}-${index}`}>
                  {blocking.item_name ? `指标“${String(blocking.item_name)}”` : blocking.task_id ? `评分任务 #${String(blocking.task_id)}` : "评分任务"}
                  {blocking.reason ? `：${String(blocking.reason)}` : blocking.status ? `：${taskLabels[String(blocking.status)] || String(blocking.status)}` : "：尚未完成"}
                </li>
              ))}
            </ul>
          </div>
        )}
        {result && (
          <div className="rounded-md border border-green-200 bg-green-50 p-3 text-sm text-green-900">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span>统一计算结果：{result.total_score.toFixed(2)} 分（{result.grade}）</span>
              <span className="text-xs text-green-700">计算版本 {result.calculation_version} · 结果 V{result.result_version}</span>
            </div>
            <div className="mt-2 grid gap-1 text-xs text-green-800">
              {result.items.map(item => (
                <div key={item.item_id} className="flex justify-between gap-3">
                  <span>{item.name}</span><span>{item.aggregated_score == null ? "未计入" : `${item.aggregated_score.toFixed(2)}（贡献 ${item.contribution.toFixed(2)}）`}</span>
                </div>
              ))}
            </div>
          </div>
        )}
        {data.tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground">当前账号没有被分配可编辑的评分任务。</p>
        ) : (
          data.tasks.map(task => (
            <TaskEditor key={task.id} task={task} busy={busyId === task.id} onSave={updateTask} labels={scorerLabels} />
          ))
        )}
      </CardContent>
    </Card>
  )
}

function TaskEditor({
  task,
  busy,
  onSave,
  labels,
}: {
  task: EvaluationScoringTaskList["tasks"][number]
  busy: boolean
  onSave: (taskId: number, score: string, comment: string, version: number, submit: boolean, notApplicable: boolean) => void
  labels: Record<string, string>
}) {
  const [score, setScore] = useState(task.score === null || task.score === undefined ? "" : String(task.score))
  const [comment, setComment] = useState(task.comment || "")
  const [notApplicable, setNotApplicable] = useState(Boolean(task.not_applicable))
  const editable = task.can_edit && task.status !== "submitted" && task.status !== "locked" && task.status !== "not_applicable"

  return (
    <div className="rounded-md border p-3 space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <div className="font-medium">{task.item_name}</div>
          <div className="text-xs text-muted-foreground">
            {labels[task.scorer_type] || task.scorer_type} · 第{task.sequence || "-"}顺位 · 满分 {task.max_score} · {task.required ? "必填" : "选填"}
          </div>
          {task.sequence_ready === false && task.status !== "submitted" && <div className="mt-1 text-xs text-amber-700">请等待前序评分人完成后再提交。</div>}
        </div>
        <Badge variant={task.status === "submitted" ? "default" : "secondary"}>{taskLabels[task.status] || task.status}</Badge>
      </div>
      <div className="grid gap-3 md:grid-cols-[160px_1fr_auto] md:items-end">
        <div className="space-y-1">
          <Label htmlFor={`task-score-${task.id}`}>评分</Label>
          <Input id={`task-score-${task.id}`} type="number" min={0} max={task.max_score} step="0.1" value={score} onChange={event => setScore(event.target.value)} disabled={!editable || busy || notApplicable} />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`task-comment-${task.id}`}>说明</Label>
          <Textarea id={`task-comment-${task.id}`} value={comment} onChange={event => setComment(event.target.value)} disabled={!editable || busy} className="min-h-[40px]" />
        </div>
        {editable && !task.required && (
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <input type="checkbox" checked={notApplicable} onChange={event => { setNotApplicable(event.target.checked); if (event.target.checked) setScore("") }} disabled={busy} />
            不适用（说明栏填写理由）
          </label>
        )}
        {editable && (
          <div className="flex gap-2">
            <Button variant="outline" size="sm" disabled={busy} onClick={() => onSave(task.id, score, comment, task.version, false, notApplicable)}>
              {busy ? <Loader2 className="mr-1 h-4 w-4 animate-spin" /> : <Save className="mr-1 h-4 w-4" />}保存
            </Button>
            <Button size="sm" disabled={busy} onClick={() => onSave(task.id, score, comment, task.version, true, notApplicable)}>提交</Button>
          </div>
        )}
      </div>
    </div>
  )
}
