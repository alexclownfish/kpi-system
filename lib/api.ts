import axios from "axios"
import { storage } from "./storage"

export interface ApiErrorPayload {
  error?: string | {
    code?: string
    message?: string
    details?: unknown
  }
  message?: string
  request_id?: string
}

export interface ApiErrorDetails {
  retryable?: boolean
  retry_guidance?: string
  result_version?: number
}

export function getApiErrorMessage(error: unknown, fallback = "操作失败，请稍后重试"): string {
  if (!axios.isAxiosError<ApiErrorPayload>(error)) {
    return error instanceof Error && error.message ? error.message : fallback
  }

  const payload = error.response?.data
  if (typeof payload?.error === "object" && payload.error?.message?.trim()) {
    return payload.error.message
  }
  if (payload?.message?.trim()) {
    return payload.message
  }
  if (typeof payload?.error === "string" && payload.error.trim()) {
    return payload.error
  }
  return error.message || fallback
}

export function getApiErrorCode(error: unknown): string | undefined {
  if (!axios.isAxiosError<ApiErrorPayload>(error)) return undefined
  const payload = error.response?.data
  return typeof payload?.error === "object" ? payload.error.code : undefined
}

export function getApiErrorDetails(error: unknown): ApiErrorDetails | undefined {
  if (!axios.isAxiosError<ApiErrorPayload>(error)) return undefined
  const details = error.response?.data?.error
  if (!details || typeof details === "string" || typeof details.details !== "object" || details.details === null) {
    return undefined
  }
  return details.details as ApiErrorDetails
}

export function getApiErrorRequestId(error: unknown): string | undefined {
  if (!axios.isAxiosError<ApiErrorPayload>(error)) return undefined
  return error.response?.data?.request_id
}

// 获取API基础URL
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api"

// 创建axios实例
const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  headers: {
    "Content-Type": "application/json",
  },
})

// 添加请求拦截器，自动附加token
api.interceptors.request.use(
  config => {
    // 添加Authorization
    const token = storage.getItem("auth_token")
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    // 添加DooTaskToken
    const dooTaskToken = storage.getItem("dootask_token")
    if (dooTaskToken) {
      config.headers.DooTaskAuth = dooTaskToken
    }
    return config
  },
  error => Promise.reject(error)
)

// 响应拦截器
api.interceptors.response.use(
  response => response.data,
  error => {
    console.error("API Error:", error)

    // 处理401错误，触发认证状态更新
    if (error.response?.status === 401) {
      if (typeof window !== "undefined") {
        window.dispatchEvent(new CustomEvent("auth_unauthorized"))
      }
    }
    return Promise.reject(error)
  }
)

// 接口类型定义
export interface Department {
  id: number
  name: string
  description: string
  parent_id?: number | null
  leader_id?: number | null
  created_at: string
  employees?: Employee[]
}

export interface Employee {
  id: number
  name: string
  email: string
  position: string
  department_id: number
  manager_id?: number
  role: string
  is_active: boolean
  created_at: string
  department?: { name: string }
  manager?: { name: string }
}

export interface DataScope {
  id: number
  code: "SELF" | "DIRECT_SUBORDINATES" | "DEPARTMENT" | "ASSIGNED" | "ALL"
  name: string
  description?: string
}

export type CreateEmployeeRequest = Omit<Employee, "id" | "created_at" | "manager_id"> & {
  manager_id?: number | null
  // 兼容历史调用点；当前员工页面会在提交前校验并始终传入。
  password?: string
}

export interface Permission {
  id: number
  code: string
  name: string
  resource: string
  action: string
  description?: string
}

export interface Role {
  id: number
  code: string
  name: string
  description?: string
  is_system: boolean
  requires_manager: boolean
  permissions?: Permission[]
  data_scope?: DataScope
  permission_count?: number
  user_count?: number
  assignable?: boolean
}

export interface RoleMutationRequest {
  name: string
  description: string
  data_scope_code: string
  requires_manager: boolean
  permission_ids: number[]
}

export interface EvaluationResponsibility {
  id: number
  code: "PM" | "DEPARTMENT_MANAGER" | "PRODUCT_MANAGER" | "CTO" | string
  name: string
  scope_type: "company" | "department" | "position"
  department_id?: number | null
  department_name?: string
  position_type?: string
  project_id?: number | null
  user_id: number
  user_name?: string
  user_email?: string
  active: boolean
  effective_from?: string | null
  effective_to?: string | null
  created_at: string
  updated_at: string
}

export type EmployeeUpdateRequest = Omit<Partial<Employee>, "manager_id"> & {
  manager_id?: number | null
  password?: string
}

export interface KPITemplate {
  id: number
  name: string
  description: string
  period: string
  department_id?: number
  position_type: string
  version: number
  status: "draft" | "published" | "retired"
  calculation_version: string
  is_active: boolean
  created_at: string
  items?: KPIItem[]
  dimensions?: KPIDimension[]
  export_layout: ResultExportLayout
  /** Source workbook provenance and publication diagnostics (Feature 013). */
  source_workbook_sha256?: string
  source_sheet?: string
  source_catalog_version?: string
  source_issues?: SourceWorkbookDiagnostic[]
  source_blocked?: boolean
  diagnostic_status?: "ok" | "warning" | "blocking" | "manual_review" | string
}

export type SourceWorkbookDiagnosticSeverity = "info" | "warning" | "blocking" | "error" | "manual_review"

export interface SourceWorkbookDiagnostic {
  severity: SourceWorkbookDiagnosticSeverity
  code: string
  sheet?: string
  cell?: string
  message: string
  repair_hint?: string
  raw_value?: unknown
  parsed_value?: unknown
  details?: unknown
}

export interface SourceWorkbookSummary {
  process_sheets: number
  position_templates: number
  hr_summary_sheets: number
  source_departments: number
  indicators: number
}

export interface SourceWorkbookPreview {
  workbook_id: string
  source_sha256: string
  parser_version: string
  sheet_count: number
  summary: SourceWorkbookSummary
  diagnostics: SourceWorkbookDiagnostic[]
  can_import_draft: boolean
  can_publish: boolean
  source_sheets?: Array<{ name: string; kind?: string; order?: number; diagnostic_status?: string }>
  template_previews?: Array<{
    source_sheet?: string
    department?: string
    position_type?: string
    period?: string
    indicator_count?: number
    diagnostic_status?: string
  }>
}

export interface SourceWorkbookImportRequest {
  source_sha256: string
  mode: "draft" | "publish"
  explicit_department_mappings?: Array<{ raw_value: string; department_id?: number; department_name?: string }>
  allow_warning_diagnostics?: boolean
}

export interface SourceWorkbookImportResponse {
  workbook_id: string
  source_sha256: string
  status?: string
  mode?: "draft" | "publish"
  parser_version?: string
  template_ids?: number[]
  template_versions?: Array<{ id: number; version: number; source_sheet?: string; status?: string }>
  created_templates: number
  reused_templates: number
  diagnostics?: SourceWorkbookDiagnostic[]
  diagnostic_summary?: Record<string, number>
}

export interface SourceWorkbookHRDiagnostic extends SourceWorkbookDiagnostic {
  period_type?: "monthly" | "quarterly" | "annual" | string
  formula?: string
  expanded_formula?: string
  cached_value?: unknown
  is_real_data_row?: boolean
}

export type ResultExportColumn =
  | "dimension_name"
  | "dimension_weight"
  | "item_name"
  | "item_description"
  | "max_score"
  | "item_weight"
  | "composite_weight"
  | "scorer_type"
  | "scorer_name"
  | "actual_scorers"
  | "weighted_contribution"
  | "self_score"
  | "self_comment"
  | "manager_score"
  | "manager_comment"
  | "hr_score"
  | "hr_comment"
  | "final_score"
  | "grade"
  | "final_comment"

export interface ResultExportLayout {
  version: number
  preset: "final_signoff" | "full_process"
  title: string
  orientation: "portrait" | "landscape"
  columns: ResultExportColumn[]
  show_summary: boolean
  show_employee_opinion: boolean
  signature_labels: string[]
}

export interface KPIItem {
  id: number
  template_id: number
  name: string
  description: string
  max_score: number
  order: number
  dimension_id?: number
  item_weight: number
  composite_weight: number
  scoring_guidance: string
  data_source: string
  scorer_types: string[]
  required: boolean
  created_at: string
}

export interface KPIDimension {
  id: number
  template_id: number
  name: string
  description: string
  weight: number
  sort_order: number
  items?: KPIItem[]
}

export interface TemplateStructureItemInput {
  name: string
  description?: string
  item_weight: number
  scoring_guidance?: string
  data_source?: string
  scorer_types: string[]
  required: boolean
  order?: number
}

export interface TemplateStructureDimensionInput {
  name: string
  description?: string
  weight: number
  sort_order?: number
  items: TemplateStructureItemInput[]
}

export interface EvaluationTemplateMatchPreview {
  workflow_mode?: "disabled" | "audit" | "enabled"
  template: KPITemplate
  scorers: Record<string, { id: number; name: string; email: string }>
  scorer_candidates?: Array<{ scorer_type: string; id: number; name: string; email: string }>
  ready?: boolean
  blocking_errors?: string[]
}

export interface KPIEvaluation {
  id: number
  employee_id: number
  template_id: number
  period: string
  year: number
  month?: number
  quarter?: number
  status: string
  total_score: number
  project_name?: string
  allocation_ratio?: number
  work_summary?: string
  work_risks?: string
  support_needed?: string
  next_plan?: string
  work_summary_required?: boolean
  work_summary_submitted_at?: string
  calculation_version?: string
  template_version?: number
  source_workbook_sha256?: string
  scorer_snapshot?: Array<{ item_id: number; scorer_type: string; user_id: number; user_name?: string; source_rule?: string }>
  signoff_snapshot?: Array<{ signer_type: string; required: boolean; sequence: number; source: string }>
  snapshot_checksum?: string
  final_comment: string
  has_objection: boolean
  objection_reason: string
  created_at: string
  employee?: Employee
  template?: KPITemplate
  scores?: KPIScore[]
}

export interface EvaluationScoringTask {
  id: number
  evaluation_id: number
  item_id: number
  item_name: string
  scorer_user_id: number
  scorer_type: string
  scorer_label?: string
  sequence?: number
  sequence_ready?: boolean
  required: boolean
  status: "pending" | "in_progress" | "submitted" | "rejected" | "not_applicable" | "locked"
  score?: number | null
  comment: string
  not_applicable: boolean
  max_score: number
  version: number
  source_rule?: string
  can_edit?: boolean
}

export interface EvaluationScoringTaskList {
  evaluation_id: number
  workflow_state: string
  completion: { required: number; submitted: number; blocking: number }
  blocking_tasks?: Array<Record<string, unknown>>
  tasks: EvaluationScoringTask[]
}

export interface WorkflowAdvanceResponse {
  previous_state: string
  current_state: string
  blocking_tasks: Array<Record<string, unknown>>
  result_version: number
}

export interface CanonicalEvaluationResult {
  evaluation_id: number
  calculation_version: string
  snapshot_checksum: string
  result_version: number
  items: Array<{
    item_id: number
    name: string
    max_score: number
    required: boolean
    composite_weight: number
    scorer_values: Array<{ task_id: number; scorer_user_id: number; scorer_type: string; score: number }>
    aggregated_score?: number
    contribution: number
    not_applicable: boolean
  }>
  total_score: number
  grade: string
  export_layout: ResultExportLayout
  calculated_at: string
}

export interface AssessmentAuditFinding {
  evaluation_id?: number
  category: string
  severity: "info" | "warning" | "error" | "manual_review"
  code: string
  message: string
  details?: unknown
  eligible_for_migration: boolean
}

export interface AssessmentAuditPreview {
  run_id: string
  summary: {
    evaluations_scanned: number
    eligible: number
    manual_review: number
    errors: number
  }
  findings: AssessmentAuditFinding[]
}

export interface AuditLogEntry {
  id: number
  action: string
  resource: string
  resource_id: string
  result: string
  details: string
  ip_address: string
  created_at: string
  actor?: { id: number; name: string; email: string }
}

export interface EvaluationResultSnapshot {
  id: number
  evaluation_id: number
  version: number
  checksum: string
  created_by: number
  created_at: string
}

export interface EvaluationConfirmation {
  id: number
  evaluation_id: number
  snapshot_id: number
  method: "online" | "paper"
  confirmed_by: number
  confirmed_at: string
  signed_at?: string
  handled_by?: number
  attachment_name?: string
  attachment_content_type?: string
  attachment_size?: number
  remark: string
  snapshot: EvaluationResultSnapshot
  confirmer?: Employee
  handler?: Employee
}

/** Paper-signature evidence uploaded after an evaluation was confirmed online. */
export interface EvaluationSignoffSupplement {
  id: number
  evaluation_id: number
  uploaded_by: number
  uploaded_at: string
  signed_at?: string
  attachment_name?: string
  attachment_content_type?: string
  attachment_size?: number
  remark: string
  uploader?: Employee
}

export interface KPIScore {
  id: number
  evaluation_id: number
  item_id: number
  self_score?: number
  self_comment: string
  manager_score?: number
  manager_comment: string
  manager_auto: boolean
  hr_score?: number
  hr_comment: string
  final_score?: number
  final_comment: string
  created_at: string
  item?: KPIItem
}

// 邀请评分相关接口
export interface EvaluationInvitation {
  id: number
  evaluation_id: number
  inviter_id: number
  invitee_id: number
  status: "pending" | "accepted" | "declined" | "completed" | "cancelled"
  message: string
  created_at: string
  updated_at: string
  evaluation?: KPIEvaluation
  inviter?: Employee
  invitee?: Employee
  scores?: InvitedScore[]
}

export interface InvitedScore {
  id: number
  invitation_id: number
  item_id: number
  score?: number
  comment: string
  created_at: string
  updated_at: string
  invitation?: EvaluationInvitation
  item?: KPIItem
}

export interface PerformanceRuleNoInvitation {
  self_weight: number
  superior_weight: number
}

export interface PerformanceRuleEmployeeInvite {
  self_weight: number
  invite_superior_weight: number
  superior_weight: number
}

export interface PerformanceRuleWithInvitation {
  employee: PerformanceRuleEmployeeInvite
}

export interface PerformanceRule {
  id: number
  no_invitation: PerformanceRuleNoInvitation
  with_invitation: PerformanceRuleWithInvitation
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface PerformanceRuleRequest {
  no_invitation: PerformanceRuleNoInvitation
  with_invitation: PerformanceRuleWithInvitation
  enabled: boolean
}

export interface DashboardStats {
  total_employees: number
  total_departments: number
  total_evaluations: number
  pending_evaluations: number
  completed_evaluations: number
  average_score: number
  recent_evaluations: KPIEvaluation[]
}

// 统计相关接口
export interface DepartmentStats {
  name: string
  total: number
  completed: number
  pending: number
  avg_score: number
}

export interface MonthlyTrend {
  month: string
  evaluations: number
  avg_score: number
  completion_rate: number
}

export interface ScoreDistribution {
  range: string
  count: number
  color: string
}

export interface TopPerformer {
  name: string
  department: string
  score: number
  evaluations: number
}

export interface RecentEvaluation {
  id: number
  employee_id?: number
  employee: string
  department: string
  template: string
  score: number
  status: string
  period: string
  year: number
  month?: number
  quarter?: number
}

// 统计数据响应类型
export interface StatisticsResponse {
  departmentStats: DepartmentStats[]
  monthlyTrends: MonthlyTrend[]
  scoreDistribution: ScoreDistribution[]
  topPerformers: TopPerformer[]
  recentEvaluations: RecentEvaluation[]
}

export interface PerformanceSummary {
  evaluation_id: number
  employee_id: number
  period_type: "monthly" | "quarterly" | "yearly"
  period: string
  total_score: number
  grade: string
  allocation_threshold: number
  allocation_ratio?: number
  weighted_contribution?: number
  evaluation_count?: number
  projects?: Array<{
    evaluation_id: number
    project_name?: string
    total_score: number
    allocation_ratio?: number
    weighted_contribution?: number
    calculation_version: string
  }>
  source_snapshot_sha256?: string
  calculation_version: string
  diagnostic_level?: "ok" | "warning" | "blocking" | string
  diagnostics?: string[]
}

// 导出响应类型
export interface ExportResponse {
  file_url: string
  file_name: string
  file_size: number
  message: string
  request_id: string
  result_version?: number
  results?: ExportResultVersion[]
  checksum?: string
}

export interface ExportResultVersion {
  evaluation_id: number
  result_version: number
  checksum: string
}

export interface FinalScoreImportRow {
  row: number
  evaluation_id?: number
  score_id?: number
  employee?: string
  item?: string
  score?: number
  comment?: string
  status: "valid" | "invalid"
  message: string
}

export interface FinalScoreImportPreview {
  message: string
  batch_id: string
  expires_at: string
  summary: {
    total_rows: number
    valid_rows: number
    invalid_rows: number
    valid_evaluations: number
    invalid_evaluations: number
  }
  rows: FinalScoreImportRow[]
}

export interface SignoffStatistics {
  summary: {
    total: number
    online: number
    paper: number
    pending: number
    received: number
    recovery_rate: number
  }
  items: Array<{
    evaluation_id: number
    employee: string
    department: string
    period: string
    total_score: number
    status: string
    method: "pending" | "online" | "paper"
    confirmed_at?: string
    handler?: string
  }>
}

// 系统设置请求类型
export interface SystemSettingsRequest {
  allow_registration: boolean
}

// 系统设置响应类型
export interface SystemSettingsResponse {
  allow_registration: boolean
  system_mode: "standalone" | "integrated" // 系统模式，独立模式: standalone，集成模式: integrated
}

// 消息类型
export interface Message {
  type: "success" | "error" | "info" | "warning" | ""
  content: string
}

// 认证相关接口类型
export interface LoginRequest {
  email: string
  password: string
}

// 用户登录（DooTaskToken）请求类型
export interface LoginByDooTaskTokenRequest {
  email: string
  token: string
}

export interface RegisterRequest {
  name: string
  email: string
  password: string
  position: string
  department_id: number
}

export interface LoginResponse {
  token: string
  user: Employee
  permissions?: string[]
  data_scope?: string
}

export interface AuthUser {
  id: number
  name: string
  email: string
  position: string
  department_id: number
  manager_id?: number
  role: string
  is_active: boolean
  created_at: string
  department?: { name: string }
  manager?: { name: string }
  permissions?: string[]
  data_scope?: string
}

// 部门API
export const departmentApi = {
  getAll: (params?: PaginationParams): Promise<PaginatedResponse<Department>> => api.get("/departments", { params }),
  getCatalog: async (): Promise<Department[]> => {
    const first = await departmentApi.getAll({ page: 1, pageSize: 100 })
    const departments = [...(first.data || [])]
    for (let page = 2; page <= first.totalPages; page++) {
      const next = await departmentApi.getAll({ page, pageSize: 100 })
      departments.push(...(next.data || []))
    }
    return departments
  },
  getPositions: (id: number): Promise<{ data: { id: number; department_id: number; name: string }[] }> => api.get(`/departments/${id}/positions`),
  getManagerCandidates: (id: number, position: string, employeeId?: number): Promise<{ data: Employee[]; message?: string }> =>
    api.get(`/departments/${id}/manager-candidates`, { params: { position, employee_id: employeeId } }),
  getById: (id: number): Promise<{ data: Department }> => api.get(`/departments/${id}`),
  create: (data: Omit<Department, "id" | "created_at">): Promise<{ data: Department }> =>
    api.post("/departments", data),
  update: (id: number, data: Partial<Department>): Promise<{ data: Department }> => api.put(`/departments/${id}`, data),
  delete: (id: number): Promise<void> => api.delete(`/departments/${id}`),
}

// 分页响应接口
export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
  totalPages: number
  hasNext: boolean
  hasPrev: boolean
}

// 评估统计数据接口
export interface EvaluationStats {
  total: number
  pending: number
  completed: number
  avgScore: number
}

// 带统计数据的评估分页响应接口
export interface EvaluationPaginatedResponse extends PaginatedResponse<KPIEvaluation> {
  stats: EvaluationStats
}

// 分页查询参数接口
export interface PaginationParams {
  page?: number
  pageSize?: number
  search?: string
  department_id?: string
  role?: string
  is_active?: boolean
}

// 评估分页查询参数接口
export interface EvaluationPaginationParams extends PaginationParams {
	assigned_to_me?: boolean
  status?: string
  employee_id?: string
  period?: string
  year?: string
  month?: string
  quarter?: string
  manager_id?: string
}

// 员工API
export const employeeApi = {
  getAll: (params?: PaginationParams): Promise<PaginatedResponse<Employee>> => api.get("/employees", { params }),
  getCatalog: async (params?: Omit<PaginationParams, "page" | "pageSize">): Promise<Employee[]> => {
    const first = await employeeApi.getAll({ ...params, page: 1, pageSize: 100 })
    const employees = [...(first.data || [])]
    for (let page = 2; page <= first.totalPages; page++) {
      const next = await employeeApi.getAll({ ...params, page, pageSize: 100 })
      employees.push(...(next.data || []))
    }
    return employees
  },
  getById: (id: number): Promise<{ data: Employee }> => api.get(`/employees/${id}`),
  create: (data: CreateEmployeeRequest): Promise<{ data: Employee; message: string }> => api.post("/employees", data),
  update: (id: number, data: EmployeeUpdateRequest): Promise<{ data: Employee }> => api.put(`/employees/${id}`, data),
  delete: (id: number): Promise<void> => api.delete(`/employees/${id}`),
  assignRole: (id: number, roleCode: string): Promise<{ message: string; role: Role }> =>
    api.put(`/employees/${id}/roles`, { role_code: roleCode }),
  getSubordinates: (id: number): Promise<{ data: Employee[]; total: number }> =>
    api.get(`/employees/${id}/subordinates`),
}

export const accessControlApi = {
  getRoles: (params?: { page?: number; pageSize?: number; search?: string; type?: "system" | "custom" }): Promise<PaginatedResponse<Role>> => api.get("/roles", { params }),
  getRole: (id: number): Promise<{ data: Role }> => api.get(`/roles/${id}`),
  createRole: (data: RoleMutationRequest): Promise<{ data: Role; message: string }> => api.post("/roles", data),
  updateRole: (id: number, data: RoleMutationRequest): Promise<{ data: Role; message: string }> => api.put(`/roles/${id}`, data),
  cloneRole: (id: number, data: { name: string; description?: string }): Promise<{ data: Role; message: string }> => api.post(`/roles/${id}/clone`, data),
  deleteRole: (id: number): Promise<{ message: string }> => api.delete(`/roles/${id}`),
  getPermissions: (): Promise<{ data: Permission[]; total: number }> => api.get("/permissions"),
  getDataScopes: (): Promise<{ data: DataScope[]; total: number }> => api.get("/data-scopes"),
  getRoleUsers: (id: number, params?: PaginationParams): Promise<PaginatedResponse<Employee>> => api.get(`/roles/${id}/users`, { params }),
  assignRoleUsers: (id: number, userIds: number[]): Promise<{ message: string }> => api.put(`/roles/${id}/users`, { user_ids: userIds }),
}

export const evaluationResponsibilityApi = {
  getAll: (params?: { code?: string; scope_type?: string; active?: boolean }): Promise<{ data: { items: EvaluationResponsibility[]; total: number } }> =>
    api.get("/evaluation-responsibilities", { params }),
  create: (data: Partial<EvaluationResponsibility> & { code: string; name: string; scope_type: string; user_id: number }): Promise<{ data: EvaluationResponsibility; message: string }> =>
    api.post("/evaluation-responsibilities", data),
  update: (id: number, data: Partial<EvaluationResponsibility> & { code: string; name: string; scope_type: string; user_id: number }): Promise<{ data: EvaluationResponsibility; message: string }> =>
    api.put(`/evaluation-responsibilities/${id}`, data),
  deactivate: (id: number): Promise<{ data: { id: number; active: boolean }; message: string }> =>
    api.delete(`/evaluation-responsibilities/${id}`),
}

// KPI模板API
export const templateApi = {
  getAll: (params?: { department_id?: number; position_type?: string; period?: string; status?: string }): Promise<{ data: KPITemplate[]; total: number }> => api.get("/templates", { params }),
  getById: (id: number): Promise<{ data: KPITemplate }> => api.get(`/templates/${id}`),
  create: (data: { name: string; description?: string; period: string; department_id?: number; position_type?: string; is_active?: boolean; export_layout?: ResultExportLayout }): Promise<{ data: KPITemplate }> =>
    api.post("/templates", data),
  update: (id: number, data: Partial<KPITemplate>): Promise<{ data: KPITemplate }> => api.put(`/templates/${id}`, data),
  saveCalculationStructure: (id: number, dimensions: TemplateStructureDimensionInput[]): Promise<{ data: KPITemplate; message: string }> =>
    api.put(`/templates/${id}/calculation-structure`, { dimensions }),
  publish: (id: number): Promise<{ data: KPITemplate; message: string }> => api.post(`/templates/${id}/publish`),
  retire: (id: number): Promise<{ message: string }> => api.post(`/templates/${id}/retire`),
  clone: (id: number, name?: string): Promise<{ data: KPITemplate; message: string }> => api.post(`/templates/${id}/clone`, { name }),
  delete: (id: number): Promise<void> => api.delete(`/templates/${id}`),
  getItems: (id: number): Promise<{ data: KPIItem[]; total: number }> => api.get(`/templates/${id}/items`),
  previewMatch: (employeeId: number, period: string): Promise<{ data: EvaluationTemplateMatchPreview }> =>
    api.get("/templates/match-preview", { params: { employee_id: employeeId, period } }),
}

/** Versioned source-workbook validation/import endpoints for HR template maintenance. */
export const sourceWorkbookApi = {
  validate: (workbook: File): Promise<{ data: SourceWorkbookPreview; request_id?: string }> => {
    const formData = new FormData()
    formData.append("workbook", workbook)
    return api.post("/source-workbooks/validate", formData, {
      headers: { "Content-Type": "multipart/form-data" },
    })
  },
  getDiagnostics: (workbookId: string): Promise<{ data: { diagnostics: SourceWorkbookDiagnostic[] }; request_id?: string }> =>
    api.get(`/source-workbooks/${encodeURIComponent(workbookId)}/diagnostics`),
  getHRDiagnostics: (workbookId: string): Promise<{ data: SourceWorkbookHRDiagnostic[]; request_id?: string }> =>
    api.get(`/source-workbooks/${encodeURIComponent(workbookId)}/hr-diagnostics`),
  importWorkbook: (
    workbookId: string,
    data: SourceWorkbookImportRequest,
  ): Promise<{ data: SourceWorkbookImportResponse; request_id?: string }> =>
    api.post(`/source-workbooks/${encodeURIComponent(workbookId)}/import`, data),
}

// KPI项目API
export const itemApi = {
  getById: (id: number): Promise<{ data: KPIItem }> => api.get(`/items/${id}`),
  create: (data: { template_id: number; name: string; description?: string; max_score?: number; order?: number; dimension_id?: number; item_weight?: number; composite_weight?: number; scoring_guidance?: string; data_source?: string; scorer_types?: string[]; required?: boolean }): Promise<{ data: KPIItem }> => api.post("/items", data),
  update: (id: number, data: Partial<KPIItem>): Promise<{ data: KPIItem }> => api.put(`/items/${id}`, data),
  delete: (id: number): Promise<void> => api.delete(`/items/${id}`),
}

// 绩效规则API
export const performanceRuleApi = {
  get: (): Promise<{ data: PerformanceRule }> => api.get("/performance-rules"),
  update: (data: PerformanceRuleRequest): Promise<{ data: PerformanceRule }> =>
    api.put("/performance-rules", data),
}

// KPI评估API
export const evaluationApi = {
  getAll: (params?: EvaluationPaginationParams): Promise<EvaluationPaginatedResponse> =>
    api.get("/evaluations", { params }),
  getById: (id: number): Promise<{ data: KPIEvaluation }> => api.get(`/evaluations/${id}`),
  create: (data: Omit<KPIEvaluation, "id" | "created_at">): Promise<{ data: KPIEvaluation }> =>
    api.post("/evaluations", data),
  update: (id: number, data: Partial<KPIEvaluation>): Promise<{ data: KPIEvaluation }> =>
    api.put(`/evaluations/${id}`, data),
  delete: (id: number): Promise<void> => api.delete(`/evaluations/${id}`),
  getByEmployee: (employeeId: number): Promise<{ data: KPIEvaluation[]; total: number }> =>
    api.get(`/evaluations/employee/${employeeId}`),
  getPending: (employeeId: number): Promise<{ data: KPIEvaluation[]; total: number }> =>
    api.get(`/evaluations/pending/${employeeId}`),
  getPendingCount: (): Promise<{ count: number }> => api.get("/evaluations/pending/count"),
  submitObjection: (id: number, data: { reason: string }): Promise<{ data: KPIEvaluation }> =>
    api.post(`/evaluations/${id}/objection`, data),
  handleObjection: (id: number, data: { total_score: number; final_comment: string }): Promise<{ data: KPIEvaluation }> =>
    api.put(`/evaluations/${id}/objection/handle`, data),
  getConfirmation: (id: number): Promise<{ data: EvaluationConfirmation | null }> =>
    api.get(`/evaluations/${id}/confirmation`),
  confirmOnline: (id: number): Promise<{ data: EvaluationConfirmation; evaluation: KPIEvaluation }> =>
    api.post(`/evaluations/${id}/confirm-online`),
  confirmPaper: (
    id: number,
    data: { signedAt: string; remark: string; file: File }
  ): Promise<{ data: EvaluationConfirmation; evaluation: KPIEvaluation }> => {
    const formData = new FormData()
    formData.append("signed_at", data.signedAt)
    formData.append("remark", data.remark)
    formData.append("file", data.file)
    return api.post(`/evaluations/${id}/confirm-paper`, formData, {
      headers: { "Content-Type": "multipart/form-data" },
    })
  },
  downloadConfirmationAttachment: (id: number): Promise<Blob> =>
    api.get(`/evaluations/${id}/confirmation/attachment`, { responseType: "blob" }),
  uploadConfirmationSupplement: (
    id: number,
    data: { signedAt?: string; remark: string; file: File },
  ): Promise<{ data: EvaluationSignoffSupplement }> => {
    const formData = new FormData()
    if (data.signedAt) formData.append("signed_at", data.signedAt)
    formData.append("remark", data.remark)
    formData.append("file", data.file)
    return api.post(`/evaluations/${id}/confirmation/supplements`, formData, {
      headers: { "Content-Type": "multipart/form-data" },
    })
  },
  listConfirmationSupplements: (id: number): Promise<{ data: EvaluationSignoffSupplement[] }> =>
    api.get(`/evaluations/${id}/confirmation/supplements`),
  downloadConfirmationSupplement: (id: number, supplementId: number): Promise<Blob> =>
    api.get(`/evaluations/${id}/confirmation/supplements/${supplementId}/attachment`, { responseType: "blob" }),
  getScoringTasks: (id: number): Promise<{ data: EvaluationScoringTaskList; message?: string }> =>
    api.get(`/evaluations/${id}/scoring-tasks`),
  submitWorkSummary: (id: number, data: { work_summary: string; work_risks: string; support_needed: string; next_plan: string }): Promise<{ data: KPIEvaluation; message?: string }> =>
    api.post(`/evaluations/${id}/work-summary`, data),
  advanceWorkflow: (id: number, expectedState: string): Promise<{ data: WorkflowAdvanceResponse; message?: string }> =>
    api.post(`/evaluations/${id}/workflow/advance`, { expected_state: expectedState }),
  getCanonicalResult: (id: number): Promise<{ data: CanonicalEvaluationResult; message?: string }> =>
    api.get(`/evaluations/${id}/result`),
}

export const assessmentAuditApi = {
  preview: (data?: {
    evaluation_ids?: number[]
    department_id?: number
    period?: string
    statuses?: string[]
    include_completed?: boolean
  }): Promise<{ data: AssessmentAuditPreview; message?: string; request_id?: string }> =>
    api.post("/assessment-audits/preview", data || {}),
  cleanupOrphanScoringTasks: (taskIds: number[]): Promise<{ data: { deleted: number; task_ids: number[] }; message?: string }> =>
    api.post("/assessment-audits/cleanup-orphan-tasks", { task_ids: taskIds, confirm: "DELETE_ORPHAN_SCORING_TASKS" }),
}

export const auditLogApi = {
  list: (params?: { action_prefix?: string; resource?: string; result?: string; limit?: number; before_id?: number }): Promise<{ data: { logs: AuditLogEntry[]; next_before_id?: number | null }; message?: string }> =>
    api.get("/audit-logs", { params }),
}

export const assessmentMigrationApi = {
  dryRun: (data: { evaluation_ids?: number[]; target_version?: string }): Promise<{ data: { batch_id: string; mode: string; eligible_evaluation_ids: number[]; manual_review: Array<Record<string, unknown>>; planned_task_count: number } }> =>
    api.post("/assessment-migrations/dry-run", data),
  apply: (batchId: string): Promise<{ data: { batch_id: string; status: string; created_task_count: number } }> =>
    api.post(`/assessment-migrations/${batchId}/apply`, { confirm_batch_id: batchId }),
  get: (batchId: string): Promise<{ data: { batch_id: string; mode: string; status: string; summary: unknown } }> =>
    api.get(`/assessment-migrations/${batchId}`),
}

// KPI评分API
export const scoreApi = {
  getByEvaluation: (evaluationId: number): Promise<{ data: KPIScore[]; total: number }> =>
    api.get(`/scores/evaluation/${evaluationId}`),
  updateSelf: (id: number, data: { self_score?: number; self_comment: string }): Promise<{ data: KPIScore }> =>
    api.put(`/scores/${id}/self`, data),
  updateManager: (id: number, data: { manager_score?: number; manager_comment: string }): Promise<{ data: KPIScore }> =>
    api.put(`/scores/${id}/manager`, data),
  updateHR: (id: number, data: { hr_score?: number; hr_comment: string }): Promise<{ data: KPIScore }> =>
    api.put(`/scores/${id}/hr`, data),
  updateFinal: (id: number, data: { final_score?: number; final_comment?: string }): Promise<{ data: KPIScore }> =>
    api.put(`/scores/${id}/final`, data),
  saveTask: (id: number, data: { score?: number | null; comment: string; not_applicable: boolean; version: number }): Promise<{ data: EvaluationScoringTask }> =>
    api.put(`/scoring-tasks/${id}`, data),
  submitTask: (id: number, data: { score?: number | null; comment: string; not_applicable: boolean; version: number }): Promise<{ data: { task: EvaluationScoringTask; workflow: { state: string; blocking_tasks: number } } }> =>
    api.post(`/scoring-tasks/${id}/submit`, data),
}

// 统计API
export const statisticsApi = {
  getDashboard: (params?: {
    year?: string
    period?: string
    month?: string
    quarter?: string
  }): Promise<{ data: DashboardStats }> => api.get("/statistics/dashboard", { params }),
  getDepartment: (id: number): Promise<StatisticsResponse> => api.get("/statistics/department/" + id),
  getEmployee: (id: number): Promise<StatisticsResponse> => api.get("/statistics/employee/" + id),
  getTrends: (period?: string): Promise<StatisticsResponse> => api.get("/statistics/trends", { params: { period } }),
  getData: (params?: {
    year?: string
    period?: string
    month?: string
    quarter?: string
  }): Promise<{ data: StatisticsResponse }> => api.get("/statistics/data", { params }),
  getPerformanceSummary: (params: {
    employee_id: number
    period_type: "monthly" | "quarterly" | "annual"
    period?: string
    allocation_ratio?: number
    allocation_threshold?: number
    diagnostics?: boolean
  }): Promise<{ data: PerformanceSummary; message?: string; request_id?: string }> => api.get("/statistics/performance-summary", { params }),
  getSignoffs: (params?: {
    department_id?: string
    year?: string
    period?: string
    month?: string
    quarter?: string
    status?: string
  }): Promise<{ data: SignoffStatistics }> => api.get("/statistics/signoffs", { params }),
}

// 导出API
export const exportApi = {
  evaluation: (id: number, format: "xlsx" | "pdf" = "xlsx"): Promise<ExportResponse> =>
    api.get(`/export/evaluation/${id}`, { params: { format } }),
  department: (id: number): Promise<ExportResponse> => api.get(`/export/department/${id}`),
  period: (period: string, params?: { year?: string; month?: string; quarter?: string }): Promise<ExportResponse> =>
    api.get(`/export/period/${period}`, { params }),
  signoffBatch: (params?: {
    department_id?: string
    year?: string
    period?: string
    month?: string
    quarter?: string
    status?: string
  }): Promise<ExportResponse> => api.get("/export/signoff-batch", { params }),
}

export const finalScoreImportApi = {
  template: (params?: {
    department_id?: string
    year?: string
    period?: string
    month?: string
    quarter?: string
  }): Promise<ExportResponse> => api.get("/final-scores/import-template", { params }),
  preview: (file: File): Promise<FinalScoreImportPreview> => {
    const form = new FormData()
    form.append("file", file)
    return api.post("/final-scores/import/preview", form, { headers: { "Content-Type": "multipart/form-data" } })
  },
  commit: (batchId: string): Promise<{
    message: string
    summary: { succeeded: number; failed: number }
    results: Array<{ evaluation_id: number; employee: string; status: string; message: string }>
  }> => api.post(`/final-scores/import/${batchId}/commit`, { valid_only: true }),
}

// 评论接口类型
export interface EvaluationComment {
  id: number
  evaluation_id: number
  user_id: number
  content: string
  is_private: boolean
  created_at: string
  updated_at: string
  user?: {
    id: number
    name: string
    email: string
    position: string
    department?: {
      name: string
    }
  }
}

// 评论API
export const commentApi = {
  getByEvaluation: (evaluationId: number, params?: PaginationParams): Promise<PaginatedResponse<EvaluationComment>> =>
    api.get(`/evaluations/${evaluationId}/comments`, { params }),
  create: (
    evaluationId: number,
    data: { content: string; is_private: boolean }
  ): Promise<{ data: EvaluationComment }> => api.post(`/evaluations/${evaluationId}/comments`, data),
  update: (
    evaluationId: number,
    commentId: number,
    data: { content: string; is_private: boolean }
  ): Promise<{ data: EvaluationComment }> => api.put(`/evaluations/${evaluationId}/comments/${commentId}`, data),
  delete: (evaluationId: number, commentId: number): Promise<void> =>
    api.delete(`/evaluations/${evaluationId}/comments/${commentId}`),
}

// 认证API
export const authApi = {
  // 用户登录
  login: (data: LoginRequest): Promise<LoginResponse> => api.post("/auth/login", data),

  // 用户登录（DooTaskToken）
  loginByDooTaskToken: (data: LoginByDooTaskTokenRequest): Promise<LoginResponse> => api.post("/auth/login-by-dootask-token", data),

  // 用户注册
  register: (data: RegisterRequest): Promise<LoginResponse> => api.post("/auth/register", data),

  // 获取当前用户信息
  getCurrentUser: (): Promise<{ data: AuthUser; permissions?: string[]; data_scope?: string }> => api.get("/me"),

  // 刷新token
  refreshToken: (): Promise<{ token: string }> => api.post("/auth/refresh"),

  // 获取部门列表（公开接口，用于注册）
  getDepartments: (): Promise<{ data: Department[] }> => api.get("/auth/departments"),

  // 登出（清除本地token）
  logout: () => {
    storage.removeItem("auth_token")
    storage.removeItem("user_info")
  },

  // 检查是否已认证
  isAuthenticated: (): boolean => {
    const token = storage.getItem("auth_token")
    return !!token
  },

  // 获取当前用户token
  getToken: (): string | null => {
    return storage.getItem("auth_token")
  },

  // 设置用户token和信息
  setAuth: (token: string, user: AuthUser) => {
    storage.setItem("auth_token", token)
    storage.setItem("user_info", JSON.stringify(user))
  },

  // 设置DooTaskToken
  setDooTaskToken: (token: string) => {
    storage.setItem("dootask_token", token)
  },

  // 获取用户信息
  getUser: (): AuthUser | null => {
    const userInfo = storage.getItem("user_info")
    return userInfo ? JSON.parse(userInfo) : null
  },
}

// 系统设置API
export const settingsApi = {
  // 获取系统设置
  get: (): Promise<{ data: SystemSettingsResponse }> => api.get("/settings"),

  // 更新系统设置
  update: (data: SystemSettingsRequest): Promise<{ data: SystemSettingsResponse; message: string }> =>
    api.put("/settings", data),
}

// 邀请评分API
export const invitationApi = {
  // 创建邀请
  create: (
    evaluationId: number,
    data: { invitee_ids: number[]; message: string }
  ): Promise<{ data: EvaluationInvitation[]; message: string }> =>
    api.post(`/evaluations/${evaluationId}/invitations`, data),

  // 获取评估的邀请列表
  getByEvaluation: (evaluationId: number): Promise<{ data: EvaluationInvitation[] }> =>
    api.get(`/evaluations/${evaluationId}/invitations`),

  // 获取我的邀请列表
  getMy: (params?: PaginationParams & { status?: string }): Promise<PaginatedResponse<EvaluationInvitation>> =>
    api.get("/invitations/my", { params }),

  // 获取我发出的邀请列表
  getSent: (params?: PaginationParams & { status?: string }): Promise<PaginatedResponse<EvaluationInvitation>> =>
    api.get("/invitations/sent", { params }),

  // 获取待确认邀请数量
  getPendingCount: (): Promise<{ count: number }> => api.get("/invitations/pending/count"),

  // 获取邀请详情
  getDetails: (invitationId: number): Promise<{ data: EvaluationInvitation }> =>
    api.get(`/invitations/${invitationId}`),

  // 接受邀请
  accept: (invitationId: number): Promise<{ data: EvaluationInvitation; message: string }> =>
    api.put(`/invitations/${invitationId}/accept`),

  // 拒绝邀请
  decline: (invitationId: number): Promise<{ data: EvaluationInvitation; message: string }> =>
    api.put(`/invitations/${invitationId}/decline`),

  // 完成邀请评分
  complete: (invitationId: number): Promise<{ data: EvaluationInvitation; message: string }> =>
    api.put(`/invitations/${invitationId}/complete`),

  // 获取邀请评分
  getScores: (invitationId: number): Promise<{ data: InvitedScore[] }> =>
    api.get(`/invitations/${invitationId}/scores`),

  // 撤销邀请
  cancel: (invitationId: number): Promise<{ data: EvaluationInvitation; message: string }> =>
    api.put(`/invitations/${invitationId}/cancel`),

  // 重新邀请
  reinvite: (invitationId: number): Promise<{ data: EvaluationInvitation; message: string }> =>
    api.put(`/invitations/${invitationId}/reinvite`),

  // 删除邀请
  delete: (invitationId: number): Promise<{ message: string }> =>
    api.delete(`/invitations/${invitationId}`),
}

// 邀请评分记录API
export const invitedScoreApi = {
  // 更新邀请评分
  update: (
    scoreId: number,
    data: { score?: number; comment: string }
  ): Promise<{ data: InvitedScore; message: string }> =>
    api.put(`/invited-scores/${scoreId}`, data),
}

// SSE状态接口
export interface SSEStatus {
  user_id: number
  is_online: boolean
  user_connection_count: number
  online_users: number
  total_connections: number
}

// 备份API类型定义
export interface BackupHistory {
  file_name: string
  file_size: number
  created_at: string
  file_type?: 'db' | 'sql' // 文件类型
  download_url: string
}

export interface BackupResponse {
  download_url: string
  file_name: string
  file_size: number
  created_at: string
  message: string
}

// 备份API
export const backupApi = {
  // 创建备份
  create: (): Promise<BackupResponse> => api.post('/backup'),

  // 获取备份历史
  getHistory: (): Promise<{ data: BackupHistory[] }> => api.get('/backup'),

  // 恢复备份
  restore: (fileName: string): Promise<{ message: string }> =>
    api.post(`/backup/restore/${fileName}`),

  // 删除备份
  delete: (fileName: string): Promise<{ message: string }> =>
    api.delete(`/backup/${fileName}`),
}

// SSE API
export const sseApi = {
  // 获取SSE状态
  getStatus: (): Promise<SSEStatus> => api.get('/events/status'),

  // 获取SSE流
  getStream: (): EventSource => {
    const token = storage.getItem("auth_token")
    if (!token) {
      throw new Error("No auth token found")
    }
    return new EventSource(`${API_BASE_URL}/events/stream?token=${encodeURIComponent(token)}`)
  },
}

export default api
