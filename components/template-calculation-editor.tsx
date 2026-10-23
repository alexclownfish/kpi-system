"use client"

import { useEffect, useMemo, useState } from "react"
import { AlertTriangle, Copy, Plus, Save, Send, Trash2, XCircle } from "lucide-react"
import { toast } from "sonner"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { getApiErrorMessage, templateApi, type KPITemplate, type TemplateStructureDimensionInput } from "@/lib/api"
import { compositeWeight, compositeWeightTotal, validateCalculationWeights } from "@/lib/template-weights"

type DraftItem = TemplateStructureDimensionInput["items"][number] & { clientId: string }
type DraftDimension = Omit<TemplateStructureDimensionInput, "items"> & { clientId: string; items: DraftItem[] }

const scorerOptions = [
  { value: "PM", label: "项目经理（PM）" },
  { value: "DEPARTMENT_MANAGER", label: "部门负责人" },
  { value: "PRODUCT_MANAGER", label: "产品经理" },
  { value: "CTO", label: "CTO" },
]

const uid = () => `${Date.now()}-${Math.random().toString(36).slice(2)}`
const newItem = (): DraftItem => ({ clientId: uid(), name: "", description: "", item_weight: 100, scoring_guidance: "", data_source: "", scorer_types: ["PM"], required: true })
const newDimension = (): DraftDimension => ({ clientId: uid(), name: "", description: "", weight: 100, items: [newItem()] })

function mapTemplate(template: KPITemplate): DraftDimension[] {
  if (!template.dimensions?.length) return [newDimension()]
  return template.dimensions.map(dimension => ({
    clientId: uid(), name: dimension.name, description: dimension.description || "", weight: dimension.weight, sort_order: dimension.sort_order,
    items: (template.items || []).filter(item => item.dimension_id === dimension.id).map(item => ({
      clientId: uid(), name: item.name, description: item.description || "", item_weight: item.item_weight, scoring_guidance: item.scoring_guidance || "", data_source: item.data_source || "", scorer_types: item.scorer_types || [], required: item.required, order: item.order,
    })),
  }))
}

export function TemplateCalculationEditor({ template, onChanged }: { template: KPITemplate; onChanged: (template: KPITemplate) => void }) {
  const [dimensions, setDimensions] = useState<DraftDimension[]>(() => mapTemplate(template))
  const [saving, setSaving] = useState(false)
  const locked = template.status === "published" || template.status === "retired"

  useEffect(() => setDimensions(mapTemplate(template)), [template])

  const dimensionTotal = useMemo(() => dimensions.reduce((sum, dimension) => sum + Number(dimension.weight || 0), 0), [dimensions])
  const compositeTotal = useMemo(() => compositeWeightTotal(dimensions), [dimensions])
  const validationErrors = useMemo(() => {
    const errors = validateCalculationWeights(dimensions)
    dimensions.forEach((dimension, index) => {
      if (!dimension.name.trim()) errors.push(`第 ${index + 1} 个维度名称不能为空`)
      if (!dimension.items.length) errors.push(`维度“${dimension.name || index + 1}”至少需要一个指标`)
      dimension.items.forEach((item, itemIndex) => {
        if (!item.name.trim()) errors.push(`维度“${dimension.name || index + 1}”的第 ${itemIndex + 1} 个指标名称不能为空`)
        if (item.required && !item.scorer_types.length) errors.push(`必填指标“${item.name || itemIndex + 1}”必须选择评分人`)
      })
    })
    return errors
  }, [dimensions])

  const updateDimension = (dimensionId: string, patch: Partial<DraftDimension>) => setDimensions(current => current.map(dimension => dimension.clientId === dimensionId ? { ...dimension, ...patch } : dimension))
  const updateItem = (dimensionId: string, itemId: string, patch: Partial<DraftItem>) => setDimensions(current => current.map(dimension => dimension.clientId === dimensionId ? { ...dimension, items: dimension.items.map(item => item.clientId === itemId ? { ...item, ...patch } : item) } : dimension))

  const save = async () => {
    if (validationErrors.length) { toast.error(validationErrors[0]); return }
    try {
      setSaving(true)
      const payload = dimensions.map((dimension, dimensionIndex) => ({
        name: dimension.name.trim(), description: dimension.description, weight: Number(dimension.weight), sort_order: dimensionIndex + 1,
        items: dimension.items.map((item, itemIndex) => ({ name: item.name.trim(), description: item.description, item_weight: Number(item.item_weight), scoring_guidance: item.scoring_guidance, data_source: item.data_source, scorer_types: item.scorer_types, required: item.required, order: itemIndex + 1 })),
      }))
      const response = await templateApi.saveCalculationStructure(template.id, payload)
      onChanged(response.data)
      toast.success(response.message)
    } catch (error) {
      toast.error(getApiErrorMessage(error, "保存计算结构失败"))
    } finally { setSaving(false) }
  }

  const publish = async () => {
    if (validationErrors.length) { toast.error(validationErrors[0]); return }
    try { const response = await templateApi.publish(template.id); onChanged(response.data); toast.success(response.message) } catch (error) { toast.error(getApiErrorMessage(error, "发布失败")) }
  }
  const clone = async () => { try { const response = await templateApi.clone(template.id, `${template.name} V${template.version + 1}`); onChanged(response.data); toast.success(response.message) } catch { toast.error("复制模板失败") } }
  const retire = async () => { try { await templateApi.retire(template.id); onChanged({ ...template, status: "retired", is_active: false }); toast.success("模板已停用") } catch { toast.error("停用模板失败") } }

  return <div className="space-y-4">
    <Card>
      <CardHeader><CardTitle className="flex flex-wrap items-center justify-between gap-3"><span>计算结构</span><div className="flex flex-wrap gap-2">
        {template.status === "draft" && <><Button variant="outline" onClick={save} disabled={saving}><Save className="mr-2 h-4 w-4" />{saving ? "保存中" : "保存结构"}</Button><Button onClick={publish} disabled={saving || validationErrors.length > 0}><Send className="mr-2 h-4 w-4" />发布模板</Button></>}
        {template.status === "published" && <Button variant="destructive" onClick={retire}><XCircle className="mr-2 h-4 w-4" />停用</Button>}
        {locked && <Button variant="outline" onClick={clone}><Copy className="mr-2 h-4 w-4" />复制新版本</Button>}
      </div></CardTitle></CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap gap-2 text-sm"><Badge variant={compositeTotal === 100 ? "default" : "destructive"}>综合权重合计 {compositeTotal.toFixed(1)}%</Badge><Badge variant="outline">维度合计 {dimensionTotal.toFixed(1)}%</Badge><Badge variant="outline">版本 V{template.version}</Badge><Badge variant="outline">{template.status === "draft" ? "草稿" : template.status === "published" ? "已发布" : "已停用"}</Badge></div>
        {locked && <p className="text-sm text-muted-foreground">已发布或停用模板保持只读；如需调整，请复制为新版本。</p>}
        {validationErrors.length > 0 && <div className="rounded-md border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive"><div className="mb-1 flex items-center font-medium"><AlertTriangle className="mr-2 h-4 w-4" />发布前需修复</div>{validationErrors.slice(0, 5).map(error => <div key={error}>• {error}</div>)}</div>}
      </CardContent>
    </Card>

    {dimensions.map((dimension, dimensionIndex) => { const itemTotal = dimension.items.reduce((sum, item) => sum + Number(item.item_weight || 0), 0); return <Card key={dimension.clientId}>
      <CardHeader><CardTitle className="flex items-center justify-between gap-3 text-base"><span>维度 {dimensionIndex + 1}</span>{!locked && dimensions.length > 1 && <Button variant="ghost" size="sm" onClick={() => setDimensions(current => current.filter(item => item.clientId !== dimension.clientId))}><Trash2 className="h-4 w-4" /></Button>}</CardTitle></CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 md:grid-cols-[1fr_160px]"><div className="space-y-2"><Label>维度名称</Label><Input disabled={locked} value={dimension.name} onChange={event => updateDimension(dimension.clientId, { name: event.target.value })} /></div><div className="space-y-2"><Label>维度占比（%）</Label><Input disabled={locked} type="number" min="0" max="100" step="0.1" value={dimension.weight} onChange={event => updateDimension(dimension.clientId, { weight: Number(event.target.value) })} /></div></div>
        <Textarea disabled={locked} placeholder="维度说明（可选）" value={dimension.description || ""} onChange={event => updateDimension(dimension.clientId, { description: event.target.value })} />
        <div className="flex items-center justify-between"><div className="font-medium">指标明细 <Badge variant="secondary">内部权重合计 {itemTotal.toFixed(1)}%</Badge></div>{!locked && <Button variant="outline" size="sm" onClick={() => updateDimension(dimension.clientId, { items: [...dimension.items, newItem()] })}><Plus className="mr-1 h-4 w-4" />添加指标</Button>}</div>
        {dimension.items.map((item, itemIndex) => <div key={item.clientId} className="space-y-3 rounded-lg border p-4">
          <div className="flex items-center justify-between"><span className="text-sm font-medium">指标 {itemIndex + 1} · 综合权重 {compositeWeight(dimension.weight, item.item_weight).toFixed(1)}%</span>{!locked && dimension.items.length > 1 && <Button variant="ghost" size="sm" onClick={() => updateDimension(dimension.clientId, { items: dimension.items.filter(current => current.clientId !== item.clientId) })}><Trash2 className="h-4 w-4" /></Button>}</div>
          <div className="grid gap-3 md:grid-cols-[1fr_160px]"><div className="space-y-2"><Label>指标名称</Label><Input disabled={locked} value={item.name} onChange={event => updateItem(dimension.clientId, item.clientId, { name: event.target.value })} /></div><div className="space-y-2"><Label>维度内权重（%）</Label><Input disabled={locked} type="number" min="0" max="100" step="0.1" value={item.item_weight} onChange={event => updateItem(dimension.clientId, item.clientId, { item_weight: Number(event.target.value) })} /></div></div>
          <Textarea disabled={locked} placeholder="指标描述" value={item.description || ""} onChange={event => updateItem(dimension.clientId, item.clientId, { description: event.target.value })} />
          <div className="grid gap-3 md:grid-cols-2"><div className="space-y-2"><Label>评分标准</Label><Textarea disabled={locked} value={item.scoring_guidance || ""} onChange={event => updateItem(dimension.clientId, item.clientId, { scoring_guidance: event.target.value })} /></div><div className="space-y-2"><Label>数据来源</Label><Textarea disabled={locked} value={item.data_source || ""} onChange={event => updateItem(dimension.clientId, item.clientId, { data_source: event.target.value })} /></div></div>
          <div className="space-y-2"><Label>评分人类型</Label><div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">{scorerOptions.map(option => <label key={option.value} className="flex items-center gap-2 rounded-md border p-2 text-sm"><Checkbox disabled={locked} checked={item.scorer_types.includes(option.value)} onCheckedChange={checked => updateItem(dimension.clientId, item.clientId, { scorer_types: checked ? [...item.scorer_types, option.value] : item.scorer_types.filter(value => value !== option.value) })} />{option.label}</label>)}</div></div>
          <label className="flex items-center gap-2 text-sm"><Checkbox disabled={locked} checked={item.required} onCheckedChange={checked => updateItem(dimension.clientId, item.clientId, { required: checked === true })} />必填指标（未完成时不能进入确认）</label>
        </div>)}
      </CardContent>
    </Card> })}
    {!locked && <Button variant="outline" className="w-full" onClick={() => setDimensions(current => [...current, newDimension()])}><Plus className="mr-2 h-4 w-4" />添加考核维度</Button>}
  </div>
}
