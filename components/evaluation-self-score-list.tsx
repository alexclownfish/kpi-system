"use client"

import { useEffect, useState } from "react"
import { Loader2, Save } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { getApiErrorMessage, scoreApi, type KPIScore } from "@/lib/api"

export function EvaluationSelfScoreList({ scores, editable, onUpdated }: {
  scores: KPIScore[]
  editable: boolean
  onUpdated: () => Promise<void>
}) {
  return <section aria-label="员工自评" className="space-y-3">
    <h3 className="text-base font-medium">员工自评</h3>
    {scores.map(score => <SelfScoreEditor key={score.id} score={score} editable={editable} onUpdated={onUpdated} />)}
  </section>
}

function SelfScoreEditor({ score, editable, onUpdated }: {
  score: KPIScore
  editable: boolean
  onUpdated: () => Promise<void>
}) {
  const [value, setValue] = useState(String(score.self_score ?? ""))
  const [comment, setComment] = useState(score.self_comment || "")
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState("")
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    setValue(String(score.self_score ?? ""))
    setComment(score.self_comment || "")
  }, [score.self_score, score.self_comment])
  const save = async () => {
    const numeric = Number(value)
    if (value.trim() === "" || !Number.isFinite(numeric) || numeric < 0 || numeric > (score.item?.max_score ?? 0)) {
      setFailed(true)
      setMessage(`请输入0到${score.item?.max_score ?? 0}之间的分数`)
      return
    }
    setBusy(true)
    setMessage("")
    try {
      await scoreApi.updateSelf(score.id, { self_score: numeric, self_comment: comment })
      await onUpdated()
      setFailed(false)
      setMessage("自评已保存")
    } catch (error) {
      setFailed(true)
      setMessage(getApiErrorMessage(error, "自评保存失败"))
    } finally { setBusy(false) }
  }
  return <div className="space-y-3 rounded-md border p-3">
    <div className="font-medium">{score.item?.name}</div>
    <div className="text-sm text-muted-foreground">满分 {score.item?.max_score} · {score.item?.required ? "必填" : "选填"}</div>
    <div className="grid gap-3 md:grid-cols-[160px_1fr_auto] md:items-end">
      <div className="space-y-1">
        <Label htmlFor={`self-score-${score.id}`}>自评分数</Label>
        <Input id={`self-score-${score.id}`} type="number" min={0} max={score.item?.max_score} step="0.1" value={value} onChange={event => setValue(event.target.value)} disabled={!editable || busy} />
      </div>
      <div className="space-y-1">
        <Label htmlFor={`self-comment-${score.id}`}>自评说明</Label>
        <Textarea id={`self-comment-${score.id}`} value={comment} onChange={event => setComment(event.target.value)} disabled={!editable || busy} />
      </div>
      {editable && <Button size="sm" disabled={busy} onClick={save}>
        {busy ? <Loader2 className="mr-1 h-4 w-4 animate-spin" /> : <Save className="mr-1 h-4 w-4" />}保存自评
      </Button>}
    </div>
    {message && <p role={failed ? "alert" : "status"} className={`text-sm ${failed ? "text-red-600" : "text-green-700"}`}>{message}</p>}
  </div>
}
