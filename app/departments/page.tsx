"use client"

import { useState, useEffect, useCallback } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { toast } from "sonner"
import { Plus, Edit, Trash2, Users, Search } from "lucide-react"
import { departmentApi, getApiErrorMessage, type Department, type PaginatedResponse, type PaginationParams } from "@/lib/api"
import { useAppContext } from "@/lib/app-context"
import { Pagination, usePagination } from "@/components/pagination"
import { LoadingInline } from "@/components/loading"
import { useAuth } from "@/lib/auth-context"

function getParentDepartmentOptions(catalog: Department[], departmentId?: number): Department[] {
  if (!departmentId) return catalog
  const excluded = new Set([departmentId])
  const pending = [departmentId]
  for (let index = 0; index < pending.length; index++) {
    for (const department of catalog) {
      if (department.parent_id === pending[index] && !excluded.has(department.id)) {
        excluded.add(department.id)
        pending.push(department.id)
      }
    }
  }
  return catalog.filter(department => !excluded.has(department.id))
}

export default function DepartmentsPage() {
  const { Confirm } = useAppContext()
  const { hasPermission } = useAuth()
  const [departments, setDepartments] = useState<Department[]>([])
  const [catalog, setCatalog] = useState<Department[]>([])
  const [formError, setFormError] = useState("")
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingDepartment, setEditingDepartment] = useState<Department | null>(null)
  const [formData, setFormData] = useState({
    name: "",
    description: "",
    parent_id: "none",
    leader_id: "none",
  })

  // 分页相关状态
  const [paginationData, setPaginationData] = useState<PaginatedResponse<Department> | null>(null)
  const [search, setSearch] = useState<string>("")

  // 使用分页Hook
  const { currentPage, pageSize, setCurrentPage, handlePageSizeChange, resetPagination } = usePagination(10)

  // 获取部门列表
  const fetchDepartments = useCallback(async () => {
    try {
      setLoading(true)

      const params: PaginationParams = {
        page: currentPage,
        pageSize: pageSize,
      }

      if (search.trim()) {
        params.search = search.trim()
      }

      const [response, allDepartments] = await Promise.all([departmentApi.getAll(params), departmentApi.getCatalog()])
      setCatalog(allDepartments)
      setDepartments(response.data || [])
      setPaginationData(response)
    } catch (error) {
      console.error("获取部门列表失败:", error)
      setFormError(getApiErrorMessage(error, "部门加载失败"))
      setDepartments([])
      setPaginationData(null)
    } finally {
      setLoading(false)
    }
  }, [currentPage, pageSize, search])

  useEffect(() => {
    fetchDepartments()
  }, [fetchDepartments])

  // 搜索处理
  const handleSearchChange = (value: string) => {
    setSearch(value)
    resetPagination()
  }

  // 创建或更新部门
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (saving) return
    setSaving(true)
    setFormError("")
    try {
      const payload = {
        ...formData,
        parent_id: formData.parent_id === "none" ? null : Number(formData.parent_id),
        leader_id: formData.leader_id === "none" ? null : Number(formData.leader_id),
      }
      if (editingDepartment) {
        await departmentApi.update(editingDepartment.id, payload)
      } else {
        await departmentApi.create(payload)
      }

      fetchDepartments()
      setDialogOpen(false)
      setEditingDepartment(null)
      setFormData({ name: "", description: "", parent_id: "none", leader_id: "none" })
      toast.success("部门保存成功")
    } catch (error) {
      console.error("保存部门失败:", error)
      setFormError(getApiErrorMessage(error, "保存部门失败"))
    } finally {
      setSaving(false)
    }
  }

  // 删除部门
  const handleDelete = async (id: number) => {
    const result = await Confirm("删除部门", "确定要删除这个部门吗？")
    if (result) {
      try {
        await departmentApi.delete(id)
        fetchDepartments()
        toast.success("部门已删除")
      } catch (error) {
        console.error("删除部门失败:", error)
        toast.error(getApiErrorMessage(error, "删除部门失败"))
      }
    }
  }

  // 打开编辑对话框
  const handleEdit = (department: Department) => {
    setEditingDepartment(department)
    setFormData({
      name: department.name,
      description: department.description,
      parent_id: department.parent_id?.toString() || "none",
      leader_id: department.leader_id?.toString() || "none",
    })
    setFormError("")
    setDialogOpen(true)
  }

  // 打开新增对话框
  const handleAdd = () => {
    setEditingDepartment(null)
    setFormData({ name: "", description: "", parent_id: "none", leader_id: "none" })
    setFormError("")
    setDialogOpen(true)
  }

  return (
    <div className="space-y-6">
      {/* 响应式头部 */}
      <div className="flex justify-between items-center flex-wrap gap-4">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold text-foreground">部门管理</h1>
          <p className="text-muted-foreground mt-1 sm:mt-2">管理组织架构和部门信息</p>
        </div>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger asChild>
            <Button onClick={handleAdd} className="w-full sm:w-auto lg:mt-8" disabled={!hasPermission("department:create")}>
              <Plus className="w-4 h-4 mr-2" />
              添加部门
            </Button>
          </DialogTrigger>
          <DialogContent className="w-[95vw] sm:max-w-md mx-auto">
            <DialogHeader>
              <DialogTitle>{editingDepartment ? "编辑部门" : "添加部门"}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <form id="department-form" onSubmit={handleSubmit} className="space-y-4">
                {formError && <p role="alert" className="text-sm text-destructive">{formError}</p>}
                <div className="flex flex-col gap-2">
                  <Label htmlFor="name">部门名称</Label>
                  <Input
                    id="name"
                    value={formData.name}
                    onChange={e => setFormData({ ...formData, name: e.target.value })}
                    required
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="description">部门描述</Label>
                  <Input
                    id="description"
                    value={formData.description}
                    onChange={e => setFormData({ ...formData, description: e.target.value })}
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="parent-department">上级部门</Label>
                  <Select value={formData.parent_id} onValueChange={parent_id => setFormData({ ...formData, parent_id })}>
                    <SelectTrigger id="parent-department" className="w-full min-w-0"><SelectValue className="min-w-0 flex-1 text-left" /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">无上级部门</SelectItem>
                      {getParentDepartmentOptions(catalog, editingDepartment?.id).map(department => (
                        <SelectItem key={department.id} value={String(department.id)} className="whitespace-normal break-words">{department.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                {editingDepartment && <div className="flex flex-col gap-2">
                  <Label htmlFor="department-leader">部门负责人</Label>
                  <Select value={formData.leader_id} onValueChange={leader_id => setFormData({ ...formData, leader_id })}>
                    <SelectTrigger id="department-leader" className="w-full min-w-0"><SelectValue className="min-w-0 flex-1 text-left [&>div]:min-w-0 [&>div]:truncate" /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">未指定</SelectItem>
                      {editingDepartment.leader_id && !editingDepartment.employees?.some(employee => employee.id === editingDepartment.leader_id) && (
                        <SelectItem value={String(editingDepartment.leader_id)} disabled>已配置负责人 #{editingDepartment.leader_id}（失效或不可见）</SelectItem>
                      )}
                      {(editingDepartment.employees || []).filter(employee => employee.is_active).map(employee => (
                        <SelectItem key={employee.id} value={String(employee.id)} className="whitespace-normal break-words">{employee.name}（{employee.position}）</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>}
              </form>
            </DialogBody>
            <DialogFooter className="flex-col-reverse sm:flex-row sm:justify-end gap-2 sm:space-x-2 sm:gap-0">
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
                className="w-full sm:w-auto"
              >
                取消
              </Button>
              <Button type="submit" form="department-form" className="w-full sm:w-auto" disabled={saving}>
                {saving ? "保存中..." : editingDepartment ? "更新" : "创建"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {!dialogOpen && formError && <p role="alert" className="text-sm text-destructive">{formError}</p>}

      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex items-center">
              <Users className="w-5 h-5 mr-2" />
              部门列表
            </div>
            <div className="flex items-center gap-4">
              <div className="relative">
                <Search className="w-4 h-4 absolute left-3 top-1/2 transform -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="搜索部门..."
                  value={search}
                  onChange={e => handleSearchChange(e.target.value)}
                  className="pl-10 w-48 sm:w-64"
                />
              </div>
            </div>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <LoadingInline className="py-8" message="加载中..." />
          ) : departments.length === 0 ? (
            <div className="text-center py-8 text-gray-500">暂无部门数据</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>部门名称</TableHead>
                  <TableHead>上级部门</TableHead>
                  <TableHead>部门负责人</TableHead>
                  <TableHead>描述</TableHead>
                  <TableHead>员工数量</TableHead>
                  <TableHead>创建时间</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {departments.map(department => (
                  <TableRow key={department.id}>
                    <TableCell className="font-medium">
                      {department.name}
                    </TableCell>
                    <TableCell>{department.parent_id ? catalog.find(parent => parent.id === department.parent_id)?.name || "未知部门" : "无上级部门"}</TableCell>
                    <TableCell>{department.leader_id ? department.employees?.find(employee => employee.id === department.leader_id)?.name || `#${department.leader_id}（失效或不可见）` : "未指定"}</TableCell>
                    <TableCell>{department.description}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{department.employees?.length || 0} 人</Badge>
                    </TableCell>
                    <TableCell>{new Date(department.created_at).toLocaleDateString()}</TableCell>
                    <TableCell className="text-right space-x-2">
                      <Button variant="outline" size="sm" onClick={() => handleEdit(department)} disabled={!hasPermission("department:edit")} title={`编辑 ${department.name}`} aria-label={`编辑 ${department.name}`}>
                        <Edit className="w-4 h-4" />
                      </Button>
                      <Button variant="outline" size="sm" onClick={() => handleDelete(department.id)} disabled={!hasPermission("department:delete")} title={`删除 ${department.name}`} aria-label={`删除 ${department.name}`}>
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}

          {/* 分页组件 */}
          {paginationData && (
            <div className="mt-6">
              <Pagination
                currentPage={currentPage}
                totalPages={paginationData.totalPages}
                pageSize={pageSize}
                totalItems={paginationData.total}
                onPageChange={setCurrentPage}
                onPageSizeChange={handlePageSizeChange}
                className="justify-center"
              />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
