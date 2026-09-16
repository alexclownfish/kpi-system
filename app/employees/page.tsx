"use client"

import { useState, useEffect, useCallback, useMemo } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Plus, Edit, Trash2, Users, Search } from "lucide-react"
import { accessControlApi, employeeApi, departmentApi, type Employee, type EmployeeUpdateRequest, type Department, type PaginatedResponse, type Role } from "@/lib/api"
import { useAppContext } from "@/lib/app-context"
import { useAuth } from "@/lib/auth-context"
import { Pagination, usePagination } from "@/components/pagination"
import { LoadingInline } from "@/components/loading"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { getRoleLabel, normalizeRoleCode, roleRequiresManager } from "@/lib/access-control"
import axios from "axios"

export default function EmployeesPage() {
  const { Alert, Confirm } = useAppContext()
  const { hasPermission } = useAuth()
  const canCreate = hasPermission("employee:create")
  const canEdit = hasPermission("employee:edit")
  const canDelete = hasPermission("employee:delete")
  const canAssignRole = hasPermission("employee:assign_role")
  const canManageSystemRoles = hasPermission("role:edit")
  const [employees, setEmployees] = useState<Employee[]>([])
  const [departments, setDepartments] = useState<Department[]>([])
  const [managers, setManagers] = useState<Employee[]>([])
  const [supervisors, setSupervisors] = useState<Employee[]>([])
  const [roles, setRoles] = useState<Role[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingEmployee, setEditingEmployee] = useState<Employee | null>(null)
  const [searchQuery, setSearchQuery] = useState("")
  const [paginationData, setPaginationData] = useState<PaginatedResponse<Employee> | null>(null)

  // 使用分页Hook
  const { currentPage, pageSize, setCurrentPage, handlePageSizeChange, resetPagination } = usePagination(10)

  const [formData, setFormData] = useState({
    name: "",
    email: "",
    password: "",
    confirmPassword: "",
    position: "",
    department_id: "",
    manager_id: "",
    role: "employee",
    is_active: true,
  })

  // 获取员工列表
  const fetchEmployees = useCallback(async () => {
    try {
      setLoading(true)
      const response = await employeeApi.getAll({
        page: currentPage,
        pageSize: pageSize,
        search: searchQuery || undefined,
      })
      setEmployees(response.data || [])
      setPaginationData(response)
    } catch (error) {
      console.error("获取员工列表失败:", error)
      setEmployees([])
      setPaginationData(null)
    } finally {
      setLoading(false)
    }
  }, [currentPage, pageSize, searchQuery])

  // 获取部门列表
  const fetchDepartments = useCallback(async () => {
    try {
      const response = await departmentApi.getAll()
      setDepartments(response.data || [])
    } catch (error) {
      console.error("获取部门列表失败:", error)
    }
  }, [])

  useEffect(() => {
    fetchEmployees()
  }, [fetchEmployees])

  useEffect(() => {
    if (canCreate || canEdit) fetchDepartments()
  }, [canCreate, canEdit, fetchDepartments])

  useEffect(() => {
    if (!canAssignRole) return
    accessControlApi.getRoles()
      .then(response => setRoles(response.data || []))
      .catch(error => console.error("获取角色列表失败:", error))
  }, [canAssignRole])

  // 搜索处理函数
  const handleSearch = useCallback(
    (value: string) => {
      setSearchQuery(value)
      resetPagination() // 搜索时重置到第一页
    },
    [resetPagination]
  )

  // 获取某个部门的上级员工
  const fetchDepartmentManagers = useCallback(async (departmentId: string) => {
    try {
      const response = await employeeApi.getAll({
        department_id: departmentId,
        role: "manager,hr",
        pageSize: 100, // 获取该部门所有上级
      })
      const departmentManagers = response.data || []
      // 如果是编辑模式，排除当前编辑的员工
      const filteredManagers = editingEmployee 
        ? departmentManagers.filter(emp => emp.id !== editingEmployee.id)
        : departmentManagers
      setManagers(filteredManagers)
    } catch (error) {
      console.error("获取部门上级失败:", error)
      setManagers([])
    }
  }, [editingEmployee])

  // 当选择部门时获取该部门的上级
  useEffect(() => {
    if (formData.department_id) {
      fetchDepartmentManagers(formData.department_id)
    } else {
      setManagers([])
    }
  }, [formData.department_id, fetchDepartmentManagers])

  // 获取可为主管设置的所有上级候选
  const fetchSupervisors = useCallback(async () => {
    try {
      const response = await employeeApi.getAll({
        pageSize: 100,
        is_active: true,
      })
      setSupervisors(response.data || [])
    } catch (error) {
      console.error("获取可选上级失败:", error)
      setSupervisors([])
    }
  }, [])

  useEffect(() => {
    if (!roleRequiresManager(formData.role) && supervisors.length === 0) {
      fetchSupervisors()
    }
  }, [formData.role, supervisors.length, fetchSupervisors])

  const supervisorOptions = useMemo(() => {
    if (!roleRequiresManager(formData.role)) {
      return supervisors.filter(emp => (editingEmployee ? emp.id !== editingEmployee.id : true))
    }
    return managers
  }, [formData.role, supervisors, managers, editingEmployee])

  const managerSelectValue = useMemo(() => {
    if (!roleRequiresManager(formData.role)) {
      return formData.manager_id || "none"
    }
    return formData.manager_id
  }, [formData.manager_id, formData.role])

  // 创建或更新员工
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      if (!formData.department_id) {
        await Alert("验证失败", "请选择员工所属部门。")
        return
      }

      if (roleRequiresManager(formData.role) && !formData.manager_id) {
        await Alert("验证失败", "普通员工必须选择直属上级，请先选择上级后再提交。")
        return
      }

      if (!editingEmployee && formData.password.length < 6) {
        await Alert("验证失败", "初始密码长度不能少于6位。")
        return
      }

      if (editingEmployee && formData.password !== "" && formData.password.length < 6) {
        await Alert("验证失败", "密码长度不能少于6位。")
        return
      }

      if (formData.password !== formData.confirmPassword) {
        await Alert("验证失败", "两次输入的密码不一致。")
        return
      }

      setSubmitting(true)

      // 构建提交数据，先处理基本字段
      const baseData = {
        name: formData.name,
        email: formData.email,
        position: formData.position,
        department_id: parseInt(formData.department_id),
        is_active: formData.is_active,
      }
      
      // 处理 manager_id：如果为空字符串，在更新时明确设置为 null，创建时设为 undefined
      const submitData = {
        ...baseData,
        ...(canAssignRole ? { role: formData.role } : {}),
        manager_id: formData.manager_id
          ? parseInt(formData.manager_id)
          : editingEmployee
            ? null
            : undefined,
        ...(formData.password ? { password: formData.password } : {}),
      }

      if (editingEmployee) {
        await employeeApi.update(editingEmployee.id, submitData as EmployeeUpdateRequest)
      } else {
        await employeeApi.create({ ...submitData, role: formData.role, password: formData.password })
      }

      await fetchEmployees()
      setDialogOpen(false)
      setEditingEmployee(null)
      setFormData({ name: "", email: "", password: "", confirmPassword: "", position: "", department_id: "", manager_id: "", role: "employee", is_active: true })
      await Alert(editingEmployee ? "更新成功" : "创建成功", editingEmployee ? "员工信息已更新。" : "新员工已创建，可使用初始密码登录。")
    } catch (error) {
      console.error("保存员工失败:", error)
      const message = axios.isAxiosError(error)
        ? error.response?.data?.error || error.response?.data?.message || error.message
        : error instanceof Error
          ? error.message
          : "未知错误"
      await Alert("保存失败", message)
    } finally {
      setSubmitting(false)
    }
  }

  // 删除员工
  const handleDelete = async (id: number) => {
    const result = await Confirm("删除员工", "确定要删除这个员工吗？")
    if (result) {
      try {
        await employeeApi.delete(id)
        fetchEmployees()
      } catch (error) {
        console.error("删除员工失败:", error)
      }
    }
  }

  // 打开编辑对话框
  const handleEdit = (employee: Employee) => {
    setEditingEmployee(employee)
    setFormData({
      name: employee.name,
      email: employee.email,
      password: "",
      confirmPassword: "",
      position: employee.position,
      department_id: employee.department_id.toString(),
      manager_id: employee.manager_id?.toString() || "",
      role: normalizeRoleCode(employee.role),
      is_active: employee.is_active,
    })
    setDialogOpen(true)
  }

  // 打开新增对话框
  const handleAdd = () => {
    setEditingEmployee(null)
    setFormData({ name: "", email: "", password: "", confirmPassword: "", position: "", department_id: "", manager_id: "", role: "employee", is_active: true })
    setDialogOpen(true)
  }

  const getRoleBadge = (role: string) => {
    const code = normalizeRoleCode(role)
    const variant = code === "super_admin" ? "destructive" : code === "employee" ? "outline" : "secondary"
    return <Badge variant={variant}>{getRoleLabel(code)}</Badge>
  }

  const assignableRoles = roles.filter(role => role.code !== "super_admin" || canManageSystemRoles)

  return (
    <div className="space-y-6">
      {/* 响应式头部 */}
      <div className="flex justify-between items-center flex-wrap gap-4">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold text-foreground">员工管理</h1>
          <p className="text-muted-foreground mt-1 sm:mt-2">管理员工信息和组织架构</p>
        </div>
        {(canCreate || canEdit) && <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          {canCreate && <DialogTrigger asChild>
            <Button onClick={handleAdd} className="w-full sm:w-auto lg:mt-8" disabled={!canCreate}>
              <Plus className="w-4 h-4 mr-2" />
              添加员工
            </Button>
          </DialogTrigger>}
          <DialogContent className="w-[95vw] sm:max-w-md mx-auto">
            <DialogHeader>
              <DialogTitle>{editingEmployee ? "编辑员工" : "添加员工"}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <form id="employee-form" onSubmit={handleSubmit} className="space-y-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="name">姓名</Label>
                  <Input
                    id="name"
                    value={formData.name}
                    onChange={e => setFormData({ ...formData, name: e.target.value })}
                    required
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="email">邮箱</Label>
                  <Input
                    id="email"
                    type="email"
                    value={formData.email}
                    onChange={e => setFormData({ ...formData, email: e.target.value })}
                    required
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="position">职位</Label>
                  <Input
                    id="position"
                    value={formData.position}
                    onChange={e => setFormData({ ...formData, position: e.target.value })}
                    required
                  />
                </div>
                {(!editingEmployee || canEdit) && (
                  <>
                    <div className="flex flex-col gap-2">
                      <Label htmlFor="password">{editingEmployee ? "新密码" : "初始密码"}</Label>
                      <Input
                        id="password"
                        type="password"
                        autoComplete="new-password"
                        value={formData.password}
                        onChange={e => setFormData({ ...formData, password: e.target.value })}
                        placeholder={editingEmployee ? "留空表示不修改" : "至少 6 位"}
                        minLength={6}
                        required={!editingEmployee}
                      />
                    </div>
                    <div className="flex flex-col gap-2">
                      <Label htmlFor="confirm-password">{editingEmployee ? "确认新密码" : "确认初始密码"}</Label>
                      <Input
                        id="confirm-password"
                        type="password"
                        autoComplete="new-password"
                        value={formData.confirmPassword}
                        onChange={e => setFormData({ ...formData, confirmPassword: e.target.value })}
                        placeholder={editingEmployee ? "再次输入新密码" : "再次输入初始密码"}
                        minLength={6}
                        required={!editingEmployee}
                      />
                    </div>
                  </>
                )}
                <div className="flex flex-col gap-2">
                  <Label htmlFor="department">部门</Label>
                  <Select
                    value={formData.department_id}
                    onValueChange={value => setFormData({ ...formData, department_id: value, manager_id: "" })}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="选择部门" />
                    </SelectTrigger>
                    <SelectContent>
                      {departments.map(dept => (
                        <SelectItem key={dept.id} value={dept.id.toString()}>
                          {dept.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                {canAssignRole && <div className="flex flex-col gap-2">
                  <Label htmlFor="role">角色</Label>
                  <Select
                    value={formData.role}
                    onValueChange={value =>
                      setFormData(prev => ({
                        ...prev,
                        role: value,
                      }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="选择角色" />
                    </SelectTrigger>
                    <SelectContent>
                      {assignableRoles.map(role => (
                        <SelectItem key={role.code} value={role.code}>{role.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>}
                <div className="flex flex-col gap-2">
                  <Label htmlFor="manager">
                    直属上级
                    {roleRequiresManager(formData.role)
                      ? "（必选）"
                      : "（可选，可指定任一员工或无上级）"}
                  </Label>
                  <Select
                    value={managerSelectValue}
                    onValueChange={value => setFormData({ ...formData, manager_id: value === "none" ? "" : value })}
                  >
                    <SelectTrigger>
                      <SelectValue
                        placeholder={
                          roleRequiresManager(formData.role)
                            ? "选择直属上级（必选）"
                            : "选择任一员工作为上级或无上级"
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {!roleRequiresManager(formData.role) && (
                        <SelectItem value="none">
                          无上级
                        </SelectItem>
                      )}
                      {supervisorOptions.length === 0 && (
                        <SelectItem value="none-disabled" disabled>
                          {roleRequiresManager(formData.role) ? "暂无可选上级" : "暂无可选员工"}
                        </SelectItem>
                      )}
                      {supervisorOptions.map(manager => {
                        const departmentText = manager.department?.name ? `（${manager.department.name}）` : ""
                        const roleText = ` - ${getRoleLabel(manager.role)}`
                        return (
                          <SelectItem key={manager.id} value={manager.id.toString()}>
                            {manager.name}
                            {departmentText}
                            {roleText}
                          </SelectItem>
                        )
                      })}
                    </SelectContent>
                  </Select>
                </div>
                {canEdit && editingEmployee && (
                  <div className="flex flex-col gap-2">
                    <Label>状态</Label>
                    <RadioGroup
                      value={formData.is_active ? "true" : "false"}
                      onValueChange={value => setFormData({ ...formData, is_active: value === "true" })}
                      className="flex flex-row gap-4"
                    >
                      <div className="flex items-center space-x-2">
                        <RadioGroupItem value="true" id="active" />
                        <Label htmlFor="active" className="font-normal cursor-pointer">
                          在职
                        </Label>
                      </div>
                      <div className="flex items-center space-x-2">
                        <RadioGroupItem value="false" id="inactive" />
                        <Label htmlFor="inactive" className="font-normal cursor-pointer">
                          离职
                        </Label>
                      </div>
                    </RadioGroup>
                  </div>
                )}
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
              <Button type="submit" form="employee-form" className="w-full sm:w-auto" disabled={submitting}>
                {submitting ? "提交中..." : editingEmployee ? "更新" : "创建"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>}
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex items-center">
              <Users className="w-5 h-5 mr-2" />
              员工列表
            </div>
            <div className="flex items-center gap-4">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-muted-foreground w-4 h-4" />
                <Input
                  placeholder="搜索员工姓名、邮箱或职位..."
                  value={searchQuery}
                  onChange={e => handleSearch(e.target.value)}
                  className="pl-10 w-48 sm:w-64"
                />
              </div>
            </div>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <LoadingInline className="py-8" message="加载中..." />
          ) : employees.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              {searchQuery ? "未找到匹配的员工" : "暂无员工数据"}
            </div>
          ) : (
            <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>姓名</TableHead>
                  <TableHead>邮箱</TableHead>
                  <TableHead>职位</TableHead>
                  <TableHead>部门</TableHead>
                  <TableHead>直属上级</TableHead>
                  <TableHead>角色</TableHead>
                  <TableHead>状态</TableHead>
                  {(canEdit || canDelete) && <TableHead className="text-right">操作</TableHead>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {employees.map(employee => (
                  <TableRow key={employee.id}>
                    <TableCell className="font-medium">{employee.name}</TableCell>
                    <TableCell>{employee.email}</TableCell>
                    <TableCell>{employee.position}</TableCell>
                    <TableCell>{employee.department?.name}</TableCell>
                    <TableCell>{employee.manager?.name || "-"}</TableCell>
                    <TableCell>{getRoleBadge(employee.role)}</TableCell>
                    <TableCell>
                      <Badge variant={employee.is_active ? "default" : "secondary"}>
                        {employee.is_active ? "在职" : "离职"}
                      </Badge>
                    </TableCell>
                    {(canEdit || canDelete) && <TableCell className="text-right space-x-2 whitespace-nowrap">
                      {canEdit && <Button variant="outline" size="sm" onClick={() => handleEdit(employee)} aria-label={`编辑 ${employee.name}`} title="编辑员工">
                        <Edit className="w-4 h-4" />
                      </Button>}
                      {canDelete && <Button variant="outline" size="sm" onClick={() => handleDelete(employee.id)} aria-label={`删除 ${employee.name}`} title="删除员工">
                        <Trash2 className="w-4 h-4" />
                      </Button>}
                    </TableCell>}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            </div>
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
