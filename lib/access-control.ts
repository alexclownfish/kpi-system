export const ROLE_LABELS: Record<string, string> = {
  super_admin: "超级管理员",
  hr: "HR 管理员",
  hr_admin: "HR 管理员",
  performance_admin: "绩效专员",
  manager: "部门负责人",
  department_manager: "部门负责人",
  reviewer: "评审人",
  analyst: "数据分析员",
  employee: "普通员工",
}

export function normalizeRoleCode(role: string) {
  if (role === "hr") return "hr_admin"
  if (role === "manager") return "department_manager"
  return role || "employee"
}

export function getRoleLabel(role: string) {
  return ROLE_LABELS[role] || role
}

export function roleRequiresManager(role: string) {
  return normalizeRoleCode(role) === "employee"
}

export function getDefaultLandingPath(permissions: string[] = []) {
  if (permissions.includes("report:company")) return "/"
  if (permissions.includes("assessment:view")) return "/evaluations"
  if (permissions.includes("review:view")) return "/invitations"
  if (permissions.includes("employee:view")) return "/employees"
  return "/settings"
}
