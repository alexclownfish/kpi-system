"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import axios from "axios"
import { Copy, Edit, Plus, Search, ShieldCheck, Trash2, Users } from "lucide-react"
import { accessControlApi, employeeApi, type DataScope, type Employee, type Permission, type Role, type RoleMutationRequest } from "@/lib/api"
import { useAppContext } from "@/lib/app-context"
import { useAuth } from "@/lib/auth-context"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"

const resourceLabels: Record<string, string> = {
  user: "用户",
  employee: "员工",
  department: "部门",
  kpi: "KPI 模板",
  assessment: "考核",
  review: "评分邀请",
  report: "报表",
  role: "角色",
  system: "系统",
  audit: "审计",
}

const emptyForm: RoleMutationRequest = {
  name: "",
  description: "",
  data_scope_code: "SELF",
  requires_manager: false,
  permission_ids: [],
}

function errorMessage(error: unknown, fallback: string) {
  if (axios.isAxiosError(error)) return error.response?.data?.error || fallback
  return fallback
}

export default function RolesPage() {
  const { Alert, Confirm } = useAppContext()
  const { user, hasPermission } = useAuth()
  const [roles, setRoles] = useState<Role[]>([])
  const [permissions, setPermissions] = useState<Permission[]>([])
  const [scopes, setScopes] = useState<DataScope[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [search, setSearch] = useState("")
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingRole, setEditingRole] = useState<Role | null>(null)
  const [form, setForm] = useState<RoleMutationRequest>(emptyForm)
  const [usersOpen, setUsersOpen] = useState(false)
  const [selectedRole, setSelectedRole] = useState<Role | null>(null)
  const [roleUsers, setRoleUsers] = useState<Employee[]>([])
  const [employeeOptions, setEmployeeOptions] = useState<Employee[]>([])
  const [selectedUserIDs, setSelectedUserIDs] = useState<number[]>([])

  const canCreate = hasPermission("role:create")
  const canEdit = hasPermission("role:edit")
  const canDelete = hasPermission("role:delete")
  const canAssign = hasPermission("employee:assign_role")

  const fetchRoles = useCallback(async () => {
    try {
      setLoading(true)
      const response = await accessControlApi.getRoles({ search: search || undefined, pageSize: 100 })
      setRoles(response.data || [])
    } catch (error) {
      await Alert("加载失败", errorMessage(error, "无法读取角色列表。"))
    } finally {
      setLoading(false)
    }
  }, [Alert, search])

  useEffect(() => {
    Promise.all([accessControlApi.getPermissions(), accessControlApi.getDataScopes()])
      .then(([permissionResponse, scopeResponse]) => {
        setPermissions(permissionResponse.data || [])
        setScopes(scopeResponse.data || [])
      })
      .catch(error => Alert("加载失败", errorMessage(error, "无法读取权限目录。")))
  }, [Alert])

  useEffect(() => {
    const timer = window.setTimeout(fetchRoles, 250)
    return () => window.clearTimeout(timer)
  }, [fetchRoles])

  const permissionGroups = useMemo(() => {
    return permissions.reduce<Record<string, Permission[]>>((groups, permission) => {
      ;(groups[permission.resource] ||= []).push(permission)
      return groups
    }, {})
  }, [permissions])

  const openCreate = () => {
    setEditingRole(null)
    setForm(emptyForm)
    setDialogOpen(true)
  }

  const openEdit = (role: Role) => {
    setEditingRole(role)
    setForm({
      name: role.name,
      description: role.description || "",
      data_scope_code: role.data_scope?.code || "SELF",
      requires_manager: role.requires_manager,
      permission_ids: role.permissions?.map(permission => permission.id) || [],
    })
    setDialogOpen(true)
  }

  const togglePermission = (permission: Permission, checked: boolean) => {
    setForm(current => ({
      ...current,
      permission_ids: checked
        ? Array.from(new Set([...current.permission_ids, permission.id]))
        : current.permission_ids.filter(id => id !== permission.id),
    }))
  }

  const submitRole = async () => {
    if (form.name.trim().length < 2) {
      await Alert("验证失败", "角色名称至少需要 2 个字符。")
      return
    }
    try {
      setSubmitting(true)
      if (editingRole) {
        await accessControlApi.updateRole(editingRole.id, form)
        await Alert("保存成功", "角色权限和数据范围已更新。")
      } else {
        await accessControlApi.createRole(form)
        await Alert("创建成功", "自定义角色已创建，可以开始分配用户。")
      }
      setDialogOpen(false)
      await fetchRoles()
    } catch (error) {
      await Alert("保存失败", errorMessage(error, "角色保存失败，请检查输入后重试。"))
    } finally {
      setSubmitting(false)
    }
  }

  const cloneRole = async (role: Role) => {
    const name = window.prompt("请输入复制后的角色名称", `${role.name} 副本`)
    if (!name) return
    try {
      await accessControlApi.cloneRole(role.id, { name, description: role.description })
      await Alert("复制成功", "已创建新的自定义角色。")
      await fetchRoles()
    } catch (error) {
      await Alert("复制失败", errorMessage(error, "角色复制失败。"))
    }
  }

  const deleteRole = async (role: Role) => {
    const confirmed = await Confirm("删除角色", `确定删除“${role.name}”吗？此操作无法撤销。`)
    if (!confirmed) return
    try {
      await accessControlApi.deleteRole(role.id)
      await Alert("删除成功", "角色已删除。")
      await fetchRoles()
    } catch (error) {
      await Alert("删除失败", errorMessage(error, "角色删除失败。"))
    }
  }

  const openUsers = async (role: Role) => {
    setSelectedRole(role)
    setSelectedUserIDs([])
    setUsersOpen(true)
    try {
      const [assigned, available] = await Promise.all([
        accessControlApi.getRoleUsers(role.id, { pageSize: 100 }),
        canAssign ? employeeApi.getAll({ pageSize: 100, is_active: true }) : Promise.resolve({ data: [] as Employee[] }),
      ])
      setRoleUsers(assigned.data || [])
      setEmployeeOptions(available.data || [])
    } catch (error) {
      await Alert("加载失败", errorMessage(error, "无法读取角色用户。"))
    }
  }

  const assignUsers = async () => {
    if (!selectedRole || selectedUserIDs.length === 0) {
      await Alert("请选择用户", "至少选择一名需要分配该角色的用户。")
      return
    }
    try {
      setSubmitting(true)
      await accessControlApi.assignRoleUsers(selectedRole.id, selectedUserIDs)
      await Alert("分配成功", `已为 ${selectedUserIDs.length} 名用户分配角色。`)
      setUsersOpen(false)
      await fetchRoles()
    } catch (error) {
      await Alert("分配失败", errorMessage(error, "角色分配失败，未写入部分结果。"))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="space-y-6 p-4 md:p-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold">用户角色</h1>
          <p className="text-sm text-muted-foreground">通过角色统一配置功能权限、数据范围和用户归属。</p>
        </div>
        {canCreate && <Button onClick={openCreate}><Plus className="mr-2 h-4 w-4" />新增角色</Button>}
      </div>

      <Card>
        <CardHeader className="gap-4 sm:flex-row sm:items-center sm:justify-between">
          <CardTitle>角色列表</CardTitle>
          <div className="relative w-full sm:w-72">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input value={search} onChange={event => setSearch(event.target.value)} placeholder="搜索角色名称或代码" className="pl-9" />
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader><TableRow><TableHead>角色</TableHead><TableHead>类型</TableHead><TableHead>数据范围</TableHead><TableHead>权限</TableHead><TableHead>用户</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {loading ? <TableRow><TableCell colSpan={6} className="py-10 text-center text-muted-foreground">正在加载角色…</TableCell></TableRow> : roles.length === 0 ? <TableRow><TableCell colSpan={6} className="py-10 text-center text-muted-foreground">暂无匹配角色</TableCell></TableRow> : roles.map(role => (
                <TableRow key={role.id}>
                  <TableCell><div className="font-medium">{role.name}</div><div className="text-xs text-muted-foreground">{role.description || role.code}</div></TableCell>
                  <TableCell><Badge variant={role.is_system ? "secondary" : "outline"}>{role.is_system ? "系统角色" : "自定义"}</Badge></TableCell>
                  <TableCell>{role.data_scope?.name || "未配置"}</TableCell>
                  <TableCell>{role.permission_count ?? role.permissions?.length ?? 0}</TableCell>
                  <TableCell>{role.user_count ?? 0}</TableCell>
                  <TableCell><div className="flex justify-end gap-1">
                    <Button variant="ghost" size="icon" title="查看用户" onClick={() => openUsers(role)}><Users className="h-4 w-4" /></Button>
                    {canCreate && <Button variant="ghost" size="icon" title="复制角色" onClick={() => cloneRole(role)}><Copy className="h-4 w-4" /></Button>}
                    {canEdit && !role.is_system && <Button variant="ghost" size="icon" title="编辑角色" onClick={() => openEdit(role)}><Edit className="h-4 w-4" /></Button>}
                    {canDelete && !role.is_system && <Button variant="ghost" size="icon" title="删除角色" onClick={() => deleteRole(role)} disabled={(role.user_count || 0) > 0}><Trash2 className="h-4 w-4 text-destructive" /></Button>}
                  </div></TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-4xl">
          <DialogHeader><DialogTitle>{editingRole ? `编辑角色：${editingRole.name}` : "新增自定义角色"}</DialogTitle></DialogHeader>
          <DialogBody className="space-y-6">
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2"><Label>角色名称</Label><Input value={form.name} maxLength={50} onChange={event => setForm(current => ({ ...current, name: event.target.value }))} placeholder="例如：绩效复核员" /></div>
              <div className="space-y-2"><Label>数据范围</Label><Select value={form.data_scope_code} onValueChange={value => setForm(current => ({ ...current, data_scope_code: value }))}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{scopes.map(scope => <SelectItem key={scope.code} value={scope.code}>{scope.name}</SelectItem>)}</SelectContent></Select></div>
              <div className="space-y-2 md:col-span-2"><Label>角色说明</Label><Textarea value={form.description} onChange={event => setForm(current => ({ ...current, description: event.target.value }))} placeholder="说明该角色的职责和适用对象" /></div>
              <div className="flex items-center justify-between rounded-lg border p-3 md:col-span-2"><div><Label>必须设置直属上级</Label><p className="text-xs text-muted-foreground">分配该角色时，用户必须已设置直属上级。</p></div><Switch checked={form.requires_manager} onCheckedChange={checked => setForm(current => ({ ...current, requires_manager: checked }))} /></div>
            </div>
            <div className="space-y-3">
              <div><Label>功能权限</Label><p className="text-xs text-muted-foreground">选择写操作时，系统会自动补充同模块的查看权限。你不能授予自己没有的权限。</p></div>
              <div className="grid gap-3 md:grid-cols-2">
                {Object.entries(permissionGroups).map(([resource, items]) => <div key={resource} className="rounded-lg border p-3"><div className="mb-2 font-medium">{resourceLabels[resource] || resource}</div><div className="space-y-2">{items.map(permission => {
                  const allowed = user?.permissions?.includes(permission.code) ?? false
                  return <label key={permission.id} className="flex items-start gap-2 text-sm"><Checkbox checked={form.permission_ids.includes(permission.id)} disabled={!allowed} onCheckedChange={checked => togglePermission(permission, checked === true)} /><span className={!allowed ? "text-muted-foreground" : ""}>{permission.name}<span className="ml-1 text-xs text-muted-foreground">{permission.code}</span></span></label>
                })}</div></div>)}
              </div>
            </div>
          </DialogBody>
          <DialogFooter><Button variant="outline" onClick={() => setDialogOpen(false)}>取消</Button><Button onClick={submitRole} disabled={submitting}>{submitting ? "正在保存…" : "保存角色"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={usersOpen} onOpenChange={setUsersOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader><DialogTitle>{selectedRole?.name} · 绑定用户</DialogTitle></DialogHeader>
          <DialogBody className="space-y-5">
            <div><div className="mb-2 text-sm font-medium">当前绑定用户（{roleUsers.length}）</div><div className="flex flex-wrap gap-2">{roleUsers.length ? roleUsers.map(employee => <Badge key={employee.id} variant="secondary">{employee.name}</Badge>) : <span className="text-sm text-muted-foreground">暂无绑定用户</span>}</div></div>
            {canAssign && <div className="space-y-2"><div className="text-sm font-medium">批量分配用户</div><div className="max-h-72 space-y-1 overflow-y-auto rounded-lg border p-2">{employeeOptions.map(employee => <label key={employee.id} className="flex items-center gap-3 rounded p-2 hover:bg-muted"><Checkbox checked={selectedUserIDs.includes(employee.id)} onCheckedChange={checked => setSelectedUserIDs(current => checked === true ? [...current, employee.id] : current.filter(id => id !== employee.id))} /><span className="flex-1 text-sm">{employee.name}<span className="ml-2 text-xs text-muted-foreground">{employee.email}</span></span></label>)}</div></div>}
          </DialogBody>
          <DialogFooter><Button variant="outline" onClick={() => setUsersOpen(false)}>关闭</Button>{canAssign && <Button onClick={assignUsers} disabled={submitting || selectedUserIDs.length === 0}><ShieldCheck className="mr-2 h-4 w-4" />{submitting ? "正在分配…" : "分配所选用户"}</Button>}</DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
