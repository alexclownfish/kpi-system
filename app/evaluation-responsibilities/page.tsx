"use client"

import { useCallback, useEffect, useState } from "react"
import axios from "axios"
import { Edit, Plus, Power, RefreshCw } from "lucide-react"
import { evaluationResponsibilityApi, departmentApi, employeeApi, type Department, type Employee, type EvaluationResponsibility } from "@/lib/api"
import { useAuth } from "@/lib/auth-context"
import { useAppContext } from "@/lib/app-context"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

const emptyForm: { code: string; name: string; scope_type: "company" | "department" | "position"; department_id: string; position_type: string; user_id: string; active: boolean } = { code: "PM", name: "项目经理", scope_type: "company", department_id: "", position_type: "", user_id: "", active: true }

function errorMessage(error: unknown) {
  if (axios.isAxiosError(error)) {
    const payload = error.response?.data as { error?: string | { message?: string } } | undefined
    if (typeof payload?.error === "string") return payload.error
    if (payload?.error && typeof payload.error === "object" && payload.error.message) return payload.error.message
  }
  return error instanceof Error ? error.message : "操作失败，请稍后重试"
}

export default function EvaluationResponsibilitiesPage() {
  const { hasPermission } = useAuth()
  const { Alert, Confirm } = useAppContext()
  const [items, setItems] = useState<EvaluationResponsibility[]>([])
  const [departments, setDepartments] = useState<Department[]>([])
  const [employees, setEmployees] = useState<Employee[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<EvaluationResponsibility | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [saving, setSaving] = useState(false)

  const canCreate = hasPermission("responsibility:create")
  const canEdit = hasPermission("responsibility:edit")
  const canDeactivate = hasPermission("responsibility:deactivate")

  const load = useCallback(async () => {
    try {
      setLoading(true)
      setLoadError(null)
      setItems([])
      setDepartments([])
      setEmployees([])
      const [responsibilities, departmentResponse, employeeResponse] = await Promise.all([
        evaluationResponsibilityApi.getAll(),
        departmentApi.getCatalog(),
        employeeApi.getCatalog({ is_active: true }),
      ])
      setItems(responsibilities.data?.items || [])
      setDepartments(departmentResponse)
      setEmployees(employeeResponse)
    } catch (error) {
      const message = errorMessage(error)
      setLoadError(message)
      await Alert("加载失败", message)
    } finally {
      setLoading(false)
    }
  }, [Alert])

  useEffect(() => { load() }, [load])

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setOpen(true)
  }

  const openEdit = (item: EvaluationResponsibility) => {
    setEditing(item)
    setForm({ code: item.code, name: item.name, scope_type: item.scope_type, department_id: item.department_id ? String(item.department_id) : "", position_type: item.position_type || "", user_id: String(item.user_id), active: item.active })
    setOpen(true)
  }

  const save = async () => {
    if (loading || loadError || employees.length === 0) return
    if (!form.name.trim() || !form.user_id) {
      await Alert("参数不完整", "请填写职责名称并选择评分人。")
      return
    }
    if (form.scope_type === "department" && !form.department_id) {
      await Alert("参数不完整", "部门作用域必须选择部门。")
      return
    }
    if (form.scope_type === "position" && !form.position_type.trim()) {
      await Alert("参数不完整", "岗位作用域必须填写岗位名称。")
      return
    }
    try {
      setSaving(true)
      const payload: { code: string; name: string; scope_type: "company" | "department" | "position"; department_id: number | null; position_type: string; user_id: number; active: boolean } = { code: form.code, name: form.name.trim(), scope_type: form.scope_type, department_id: form.department_id ? Number(form.department_id) : null, position_type: form.position_type.trim(), user_id: Number(form.user_id), active: form.active }
      if (editing) await evaluationResponsibilityApi.update(editing.id, payload)
      else await evaluationResponsibilityApi.create(payload)
      setOpen(false)
      await Alert("保存成功", "职责配置已保存；只会影响后续新建考核，已创建考核不变。")
      await load()
    } catch (error) {
      await Alert("保存失败", errorMessage(error))
    } finally {
      setSaving(false)
    }
  }

  const deactivate = async (item: EvaluationResponsibility) => {
    if (!(await Confirm("停用评分职责", `确定停用“${item.name} - ${item.user_name || item.user_id}”吗？`))) return
    try {
      await evaluationResponsibilityApi.deactivate(item.id)
      await Alert("已停用", "已创建考核的评分任务不会被改写。")
      await load()
    } catch (error) {
      await Alert("停用失败", errorMessage(error))
    }
  }

  return (
    <div className="space-y-6 p-4 md:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div><h1 className="text-2xl font-bold">评分职责</h1><p className="text-sm text-muted-foreground">将项目经理、部门负责人等业务评分职责绑定到具体用户和作用域，不依赖系统角色名称。</p></div>
        <div className="flex gap-2"><Button variant="outline" disabled={loading || saving} onClick={load}><RefreshCw className="mr-2 h-4 w-4" />刷新</Button>{canCreate && <Button disabled={loading || !!loadError} onClick={openCreate}><Plus className="mr-2 h-4 w-4" />新增职责</Button>}</div>
      </div>
      <Card><CardHeader><CardTitle>当前配置</CardTitle></CardHeader><CardContent>
        <Table><TableHeader><TableRow><TableHead>职责</TableHead><TableHead>作用域</TableHead><TableHead>评分人</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>
          {loading ? <TableRow><TableCell colSpan={5}>正在加载…</TableCell></TableRow> : loadError ? <TableRow><TableCell colSpan={5} role="alert" className="text-destructive">加载失败：{loadError}。请点击刷新重试。</TableCell></TableRow> : items.length === 0 ? <TableRow><TableCell colSpan={5}>暂无评分职责，请先新增。</TableCell></TableRow> : items.map(item => <TableRow key={item.id}><TableCell><div className="font-medium">{item.name}</div><div className="text-xs text-muted-foreground">{item.code}</div></TableCell><TableCell>{item.scope_type === "company" ? "全公司" : item.scope_type === "department" ? `部门：${item.department_name || item.department_id}` : `岗位：${item.position_type}`}</TableCell><TableCell>{item.user_name || item.user_id}<div className="text-xs text-muted-foreground">{item.user_email}</div></TableCell><TableCell>{item.active ? "启用" : "已停用"}</TableCell><TableCell className="text-right"><div className="flex justify-end gap-2">{canEdit && <Button size="sm" variant="outline" disabled={loading || !!loadError} onClick={() => openEdit(item)}><Edit className="mr-1 h-4 w-4" />编辑</Button>}{canDeactivate && item.active && <Button size="sm" variant="outline" onClick={() => deactivate(item)}><Power className="mr-1 h-4 w-4" />停用</Button>}</div></TableCell></TableRow>)}
        </TableBody></Table>
      </CardContent></Card>

      <Dialog open={open} onOpenChange={setOpen}><DialogContent><DialogHeader><DialogTitle>{editing ? "编辑评分职责" : "新增评分职责"}</DialogTitle></DialogHeader><DialogBody><div className="grid gap-4">
        <div><Label>职责类型</Label><Select value={form.code} onValueChange={value => setForm(current => ({ ...current, code: value, name: value === "PM" ? "项目经理" : value === "DEPARTMENT_MANAGER" ? "部门负责人" : value === "PRODUCT_MANAGER" ? "产品经理" : "CTO" }))}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="PM">项目经理</SelectItem><SelectItem value="DEPARTMENT_MANAGER">部门负责人</SelectItem><SelectItem value="PRODUCT_MANAGER">产品经理</SelectItem><SelectItem value="CTO">CTO</SelectItem></SelectContent></Select></div>
        <div><Label>显示名称</Label><Input value={form.name} onChange={event => setForm(current => ({ ...current, name: event.target.value }))} /></div>
        <div><Label>作用域</Label><Select value={form.scope_type} onValueChange={value => setForm(current => ({ ...current, scope_type: value as "company" | "department" | "position" }))}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="company">全公司</SelectItem><SelectItem value="department">指定部门</SelectItem><SelectItem value="position">指定岗位</SelectItem></SelectContent></Select></div>
        {form.scope_type === "department" && <div><Label>部门</Label><Select value={form.department_id} onValueChange={value => setForm(current => ({ ...current, department_id: value }))}><SelectTrigger><SelectValue placeholder="选择部门" /></SelectTrigger><SelectContent>{departments.map(department => <SelectItem key={department.id} value={String(department.id)}>{department.name}</SelectItem>)}</SelectContent></Select></div>}
        {form.scope_type === "position" && <div><Label>岗位名称</Label><Input value={form.position_type} onChange={event => setForm(current => ({ ...current, position_type: event.target.value }))} placeholder="例如：高级工程师" /></div>}
        <div className="min-w-0"><Label>评分人</Label><Select disabled={loading || !!loadError || employees.length === 0} value={form.user_id} onValueChange={value => setForm(current => ({ ...current, user_id: value }))}><SelectTrigger className="w-full min-w-0"><SelectValue className="block! min-w-0 flex-1 truncate" placeholder={loading ? "正在加载员工…" : loadError ? "员工加载失败" : employees.length === 0 ? "暂无可选启用员工" : "选择启用员工"} /></SelectTrigger><SelectContent className="max-w-[calc(100vw-2rem)]">{employees.map(employee => <SelectItem className="break-all [&>span:last-child]:block" key={employee.id} value={String(employee.id)}>{employee.name}（{employee.email}）</SelectItem>)}</SelectContent></Select></div>
        <div className="flex items-center justify-between"><Label>启用</Label><Switch checked={form.active} onCheckedChange={checked => setForm(current => ({ ...current, active: checked }))} /></div>
      </div></DialogBody><DialogFooter><Button variant="outline" onClick={() => setOpen(false)}>取消</Button><Button disabled={saving || loading || !!loadError || employees.length === 0} onClick={save}>{saving ? "保存中…" : "保存"}</Button></DialogFooter></DialogContent></Dialog>
    </div>
  )
}
