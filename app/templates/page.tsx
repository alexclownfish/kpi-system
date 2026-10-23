"use client"

import { useState, useEffect, useCallback } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Plus, Edit, Trash2, ClipboardList, Settings, Eye, ArrowUp, ArrowDown, Save, X, Copy, Upload, RefreshCw, FileSearch, AlertTriangle, CheckCircle2 } from "lucide-react"
import { departmentApi, templateApi, itemApi, sourceWorkbookApi, getApiErrorMessage, getApiErrorRequestId, type Department, type KPITemplate, type KPIItem, type ResultExportColumn, type ResultExportLayout, type SourceWorkbookDiagnostic, type SourceWorkbookHRDiagnostic, type SourceWorkbookImportResponse, type SourceWorkbookPreview } from "@/lib/api"
import { TemplateCalculationEditor } from "@/components/template-calculation-editor"
import { templateScoreMaximum } from "@/lib/template-weights"
import { useAppContext } from "@/lib/app-context"
import { getPeriodLabel, formatScore } from "@/lib/utils"
import { LoadingInline } from "@/components/loading"
import { toast } from "sonner"

const defaultTemplateFormData = {
  name: "",
  description: "",
  period: "monthly",
  department_id: 0,
  position_type: "",
  is_active: true,
}

const defaultItemFormData = {
  name: "",
  description: "",
  max_score: 0,
  order: 1,
}

const templateDisplayName = (name: string) => name.replace(/(?:\s+V\d+){2,}$/, "").trim()

const exportFields: Array<{ key: ResultExportColumn; label: string; required?: boolean }> = [
  { key: "dimension_name", label: "维度" },
  { key: "dimension_weight", label: "维度权重" },
  { key: "item_name", label: "指标", required: true },
  { key: "item_description", label: "指标说明" },
  { key: "max_score", label: "满分" },
  { key: "item_weight", label: "指标权重" },
  { key: "composite_weight", label: "综合权重" },
  { key: "scorer_type", label: "评分职责" },
  { key: "scorer_name", label: "实际评分人" },
  { key: "weighted_contribution", label: "加权贡献" },
  { key: "self_score", label: "自评分" },
  { key: "self_comment", label: "自评说明" },
  { key: "manager_score", label: "主管评分" },
  { key: "manager_comment", label: "主管说明" },
  { key: "hr_score", label: "HR评分" },
  { key: "hr_comment", label: "HR说明" },
  { key: "final_score", label: "最终得分", required: true },
  { key: "grade", label: "评分等级" },
  { key: "final_comment", label: "最终评价" },
]

const exportPresets: Record<ResultExportLayout["preset"], ResultExportLayout> = {
  final_signoff: {
    version: 1,
    preset: "final_signoff",
    title: "绩效考核结果确认表",
    orientation: "portrait",
    columns: ["item_name", "max_score", "weighted_contribution", "final_score", "grade", "final_comment"],
    show_summary: true,
    show_employee_opinion: true,
    signature_labels: ["员工签字", "直属主管签字", "HR签字", "签字日期"],
  },
  full_process: {
    version: 1,
    preset: "full_process",
    title: "绩效考核结果确认表",
    orientation: "portrait",
    columns: exportFields.map(field => field.key),
    show_summary: true,
    show_employee_opinion: true,
    signature_labels: ["员工签字", "直属主管签字", "HR签字", "签字日期"],
  },
}

export default function TemplatesPage() {
  const { Alert, Confirm } = useAppContext()
  const [templates, setTemplates] = useState<KPITemplate[]>([])
  const [departments, setDepartments] = useState<Department[]>([])
  const [selectedTemplate, setSelectedTemplate] = useState<KPITemplate | null>(null)
  const [templateTabValue, setTemplateTabValue] = useState("templates")
  const [items, setItems] = useState<KPIItem[]>([])
  const [loading, setLoading] = useState(true)
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false)
  const [itemDialogOpen, setItemDialogOpen] = useState(false)
  const [editingTemplate, setEditingTemplate] = useState<KPITemplate | null>(null)
  const [editingItem, setEditingItem] = useState<KPIItem | null>(null)
  const [templateFormData, setTemplateFormData] = useState(defaultTemplateFormData)
  const [itemFormData, setItemFormData] = useState(defaultItemFormData)
  const [exportLayout, setExportLayout] = useState<ResultExportLayout>(exportPresets.final_signoff)
  const [savingExportLayout, setSavingExportLayout] = useState(false)
  const [cloningTemplate, setCloningTemplate] = useState(false)
  const exportLayoutLocked = selectedTemplate?.status === "published" || selectedTemplate?.status === "retired"
  const [filterDepartmentId, setFilterDepartmentId] = useState<number | undefined>()
  const [filterPositionType, setFilterPositionType] = useState("")
  const [filterPeriod, setFilterPeriod] = useState("")
  const [showHistoricalTemplates, setShowHistoricalTemplates] = useState(false)
  const [sourceFile, setSourceFile] = useState<File | null>(null)
  const [sourcePreview, setSourcePreview] = useState<SourceWorkbookPreview | null>(null)
  const [sourceDiagnostics, setSourceDiagnostics] = useState<SourceWorkbookDiagnostic[]>([])
  const [sourceHRDiagnostics, setSourceHRDiagnostics] = useState<SourceWorkbookHRDiagnostic[]>([])
  const [sourceValidating, setSourceValidating] = useState(false)
  const [sourceLoadingDiagnostics, setSourceLoadingDiagnostics] = useState(false)
  const [sourceLoadingHRDiagnostics, setSourceLoadingHRDiagnostics] = useState(false)
  const [sourceImporting, setSourceImporting] = useState<"draft" | "publish" | null>(null)
  const [sourceImportResult, setSourceImportResult] = useState<SourceWorkbookImportResponse | null>(null)

  const clearSourceResult = () => {
    setSourcePreview(null)
    setSourceDiagnostics([])
    setSourceHRDiagnostics([])
    setSourceImportResult(null)
  }

  const sourceBlockingCount = (sourcePreview?.diagnostics || sourceDiagnostics)
    .filter(diagnostic => diagnostic.severity === "blocking" || diagnostic.severity === "error").length
  const sourceWarningCount = sourceDiagnostics.filter(diagnostic => diagnostic.severity === "warning").length

  const validateSourceWorkbook = async () => {
    if (!sourceFile || sourceValidating || sourceImporting) return
    setSourceValidating(true)
    clearSourceResult()
    try {
      const response = await sourceWorkbookApi.validate(sourceFile)
      const preview = response.data
      setSourcePreview(preview)
      setSourceDiagnostics(preview.diagnostics || [])
      toast.success(`校验完成：${preview.summary.position_templates} 套岗位模板，${preview.summary.indicators} 个指标`)
    } catch (error) {
      clearSourceResult()
      const requestId = getApiErrorRequestId(error)
      await Alert("工作簿校验失败", `${getApiErrorMessage(error, "无法解析工作簿，请修正文件后重试。")}${requestId ? `（请求标识：${requestId}）` : ""}`)
    } finally {
      setSourceValidating(false)
    }
  }

  const loadSourceDiagnostics = async () => {
    if (!sourcePreview || sourceLoadingDiagnostics || sourceImporting) return
    setSourceLoadingDiagnostics(true)
    try {
      const response = await sourceWorkbookApi.getDiagnostics(sourcePreview.workbook_id)
      const detail = response.data
      setSourcePreview(current => current ? { ...current, diagnostics: detail.diagnostics || [] } : null)
      setSourceDiagnostics(detail.diagnostics || [])
    } catch (error) {
      clearSourceResult()
      const requestId = getApiErrorRequestId(error)
      await Alert("读取诊断失败", `${getApiErrorMessage(error, "读取来源诊断失败，请重新校验。")}${requestId ? `（请求标识：${requestId}）` : ""}`)
    } finally {
      setSourceLoadingDiagnostics(false)
    }
  }

  const loadSourceHRDiagnostics = async () => {
    if (!sourcePreview || sourceLoadingHRDiagnostics || sourceImporting) return
    setSourceLoadingHRDiagnostics(true)
    try {
      const response = await sourceWorkbookApi.getHRDiagnostics(sourcePreview.workbook_id)
      setSourceHRDiagnostics(response.data || [])
    } catch (error) {
      setSourceHRDiagnostics([])
      const requestId = getApiErrorRequestId(error)
      await Alert("读取 HR 诊断失败", `${getApiErrorMessage(error, "读取 HR 汇总诊断失败，请重试。")}${requestId ? `（请求标识：${requestId}）` : ""}`)
    } finally {
      setSourceLoadingHRDiagnostics(false)
    }
  }

  const importSourceWorkbook = async (mode: "draft" | "publish") => {
    if (!sourcePreview || sourceImporting || sourceImportResult || sourceLoadingDiagnostics || sourceLoadingHRDiagnostics) return
    if (mode === "publish" && (sourceBlockingCount > 0 || !sourcePreview.can_publish)) return
    setSourceImporting(mode)
    try {
      if (mode === "publish" && sourceWarningCount > 0) {
        const accepted = await Confirm("确认带告警发布", `当前有 ${sourceWarningCount} 项告警，未发现阻断问题。系统会切换为新的当前使用版本，旧版本保留为历史版本；历史考核不会改变。确认已查看诊断并继续发布？`)
        if (!accepted) return
      }
      const response = await sourceWorkbookApi.importWorkbook(sourcePreview.workbook_id, {
        source_sha256: sourcePreview.source_sha256,
        mode,
        explicit_department_mappings: [],
        allow_warning_diagnostics: true,
      })
      setSourceImportResult(response.data)
      setSourcePreview(current => current ? { ...current, can_import_draft: false, can_publish: false } : null)
      toast.success(`导入完成：新增 ${response.data.created_templates} 套，复用 ${response.data.reused_templates} 套模板；旧版本保留为历史`)
      await fetchTemplates()
      if (response.data?.diagnostics) setSourceDiagnostics(response.data.diagnostics)
    } catch (error) {
      // An import failure must not leave a partially displayed result that users could publish.
      clearSourceResult()
      const requestId = getApiErrorRequestId(error)
      await Alert("导入失败", `${getApiErrorMessage(error, "工作簿导入失败，未保存任何模板。请修正后重新校验。")}${requestId ? `（请求标识：${requestId}）` : ""}`)
    } finally {
      setSourceImporting(null)
    }
  }

  // 获取模板列表
  const fetchTemplates = useCallback(async () => {
    try {
      const response = await templateApi.getAll({
        department_id: filterDepartmentId,
        position_type: filterPositionType || undefined,
        period: filterPeriod || undefined,
      })
      setTemplates(response.data || [])
    } catch (error) {
      console.error("获取模板列表失败:", error)
    }
    setLoading(false)
  }, [filterDepartmentId, filterPositionType, filterPeriod])

  const fetchDepartments = async () => {
    try {
      setDepartments(await departmentApi.getCatalog())
    } catch (error) {
      console.error("获取部门列表失败:", error)
    }
  }

  // 获取模板的KPI项目
  const fetchTemplateItems = async (templateId: number) => {
    try {
      const response = await templateApi.getItems(templateId)
      setItems(response.data || [])
    } catch (error) {
      console.error("获取KPI项目失败:", error)
      setItems([])
    }
  }

  useEffect(() => { fetchDepartments() }, [])
  useEffect(() => { fetchTemplates() }, [fetchTemplates])
  const visibleTemplates = templates.filter(template => showHistoricalTemplates || template.status !== "retired")

  // 创建或更新模板
  const handleTemplateSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      if (editingTemplate) {
        await templateApi.update(editingTemplate.id, templateFormData)
      } else {
        await templateApi.create(templateFormData)
      }

      fetchTemplates()
      setTemplateDialogOpen(false)
      setEditingTemplate(null)
      setTemplateFormData(defaultTemplateFormData)
    } catch (error) {
      console.error("保存模板失败:", error)
    }
  }

  // 创建或更新KPI项目
  const handleItemSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedTemplate) return

    try {
      const submitData = {
        ...itemFormData,
        template_id: selectedTemplate.id,
      }

      if (editingItem) {
        await itemApi.update(editingItem.id, submitData)
      } else {
        await itemApi.create(submitData)
      }

      fetchTemplateItems(selectedTemplate.id)
      setItemDialogOpen(false)
      setEditingItem(null)
      setItemFormData(defaultItemFormData)
    } catch (error) {
      console.error("保存KPI项目失败:", error)
    }
  }

  // 删除模板
  const handleDeleteTemplate = async (id: number) => {
    const result = await Confirm("删除模板", "确定要删除这个模板吗？")
    if (result) {
      try {
        await templateApi.delete(id)
        fetchTemplates()
        if (selectedTemplate?.id === id) {
          setSelectedTemplate(null)
          setItems([])
        }
      } catch (error) {
        console.error("删除模板失败:", error)
      }
    }
  }

  // 删除KPI项目
  const handleDeleteItem = async (id: number) => {
    const result = await Confirm("删除KPI项目", "确定要删除这个KPI项目吗？")
    if (result) {
      try {
        await itemApi.delete(id)
        if (selectedTemplate) {
          fetchTemplateItems(selectedTemplate.id)
        }
      } catch (error) {
        console.error("删除KPI项目失败:", error)
      }
    }
  }

  // 选择模板并加载其KPI项目
  const handleSelectTemplate = (template: KPITemplate) => {
    setSelectedTemplate(template)
    setExportLayout(template.export_layout || exportPresets.final_signoff)
    fetchTemplateItems(template.id)
    setTemplateTabValue("items")
  }

  const saveExportLayout = async () => {
    if (!selectedTemplate || exportLayoutLocked || savingExportLayout) return
    try {
      setSavingExportLayout(true)
      const response = await templateApi.update(selectedTemplate.id, { export_layout: exportLayout })
      setSelectedTemplate(response.data)
      setExportLayout(response.data.export_layout)
      setTemplates(current => current.map(template => template.id === response.data.id ? response.data : template))
      toast.success("结果导出版式已保存")
    } catch (error) {
      await Alert("保存失败", getApiErrorMessage(error, "结果导出版式保存失败，请检查配置后重试。"))
    } finally {
      setSavingExportLayout(false)
    }
  }

  const cloneSelectedTemplate = async () => {
    if (!selectedTemplate || cloningTemplate) return
    try {
      setCloningTemplate(true)
      const response = await templateApi.clone(selectedTemplate.id, `${templateDisplayName(selectedTemplate.name)} V${(selectedTemplate.version || 1) + 1}`)
      setSelectedTemplate(response.data)
      setExportLayout(response.data.export_layout || exportPresets.final_signoff)
      setTemplates(current => [...current, response.data])
      await fetchTemplateItems(response.data.id)
      toast.success(response.message || "模板已复制为新版本")
    } catch (error) {
      await Alert("复制失败", getApiErrorMessage(error, "复制模板失败，请稍后重试。"))
    } finally {
      setCloningTemplate(false)
    }
  }

  const toggleExportColumn = (key: ResultExportColumn, checked: boolean) => {
    const field = exportFields.find(item => item.key === key)
    if (field?.required && !checked) return
    setExportLayout(current => ({
      ...current,
      columns: checked ? [...current.columns, key] : current.columns.filter(column => column !== key),
    }))
  }

  const moveExportColumn = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= exportLayout.columns.length) return
    const columns = [...exportLayout.columns]
    ;[columns[index], columns[target]] = [columns[target], columns[index]]
    setExportLayout(current => ({ ...current, columns }))
  }

  // 打开编辑模板对话框
  const handleEditTemplate = (template: KPITemplate) => {
    setEditingTemplate(template)
    setTemplateFormData({
      name: template.name,
      description: template.description,
      period: template.period,
      department_id: template.department_id || 0,
      position_type: template.position_type || "",
      is_active: template.is_active,
    })
    setTemplateDialogOpen(true)
  }

  // 打开编辑KPI项目对话框
  const handleEditItem = (item: KPIItem) => {
    setEditingItem(item)
    setItemFormData({
      name: item.name,
      description: item.description,
      max_score: item.max_score,
      order: item.order,
    })
    setItemDialogOpen(true)
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center flex-wrap gap-4">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold text-foreground">KPI模板管理</h1>
          <p className="text-muted-foreground mt-1 sm:mt-2">创建和管理KPI考核模板</p>
        </div>
        <Dialog open={templateDialogOpen} onOpenChange={setTemplateDialogOpen}>
          <DialogTrigger asChild>
            <Button className="w-full sm:w-auto lg:mt-8" onClick={() => {
              setEditingTemplate(null)
              setTemplateFormData(defaultTemplateFormData)
            }}>
              <Plus className="w-4 h-4 mr-2" />
              创建模板
            </Button>
          </DialogTrigger>
          <DialogContent className="w-[95vw] sm:max-w-md mx-auto">
            <DialogHeader>
              <DialogTitle>{editingTemplate ? "编辑模板" : "创建模板"}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <form id="template-form" onSubmit={handleTemplateSubmit} className="space-y-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="name">模板名称</Label>
                  <Input
                    id="name"
                    value={templateFormData.name}
                    onChange={e => setTemplateFormData({ ...templateFormData, name: e.target.value })}
                    required
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="description">模板描述</Label>
                  <Input
                    id="description"
                    value={templateFormData.description}
                    onChange={e => setTemplateFormData({ ...templateFormData, description: e.target.value })}
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="period">考核周期</Label>
                  <Select
                    value={templateFormData.period}
                    onValueChange={value => setTemplateFormData({ ...templateFormData, period: value })}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="选择考核周期" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="monthly">月度</SelectItem>
                      <SelectItem value="quarterly">季度</SelectItem>
                      <SelectItem value="yearly">年度</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="department">适用部门</Label>
                  <Select
                    value={templateFormData.department_id ? String(templateFormData.department_id) : "none"}
                    onValueChange={value => setTemplateFormData({ ...templateFormData, department_id: value === "none" ? 0 : Number(value) })}
                  >
                    <SelectTrigger id="department"><SelectValue placeholder="选择部门" /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">未指定（通用草稿）</SelectItem>
                      {departments.map(department => <SelectItem key={department.id} value={String(department.id)}>{department.name}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="position-type">适用岗位类型</Label>
                  <Input
                    id="position-type"
                    placeholder="例如：研发工程师、部门负责人"
                    value={templateFormData.position_type}
                    onChange={e => setTemplateFormData({ ...templateFormData, position_type: e.target.value })}
                  />
                </div>
              </form>
            </DialogBody>
            <DialogFooter className="flex-col-reverse sm:flex-row sm:justify-end gap-2 sm:space-x-2 sm:gap-0">
              <Button
                type="button"
                variant="outline"
                onClick={() => setTemplateDialogOpen(false)}
                className="w-full sm:w-auto"
              >
                取消
              </Button>
              <Button type="submit" form="template-form" className="w-full sm:w-auto">
                {editingTemplate ? "更新" : "创建"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <Tabs className="w-full" value={templateTabValue} onValueChange={setTemplateTabValue}>
        <TabsList className="gap-1 mb-2 max-w-full overflow-x-auto justify-start">
          <TabsTrigger value="templates">模板列表</TabsTrigger>
          <TabsTrigger value="source-import">导入新版工作簿</TabsTrigger>
          {selectedTemplate && (
            <TabsTrigger value="items">KPI项目 {selectedTemplate && `(${selectedTemplate.name})`}</TabsTrigger>
          )}
          {selectedTemplate && <TabsTrigger value="calculation-structure">计算结构</TabsTrigger>}
          {selectedTemplate && <TabsTrigger value="export-layout">结果导出设置</TabsTrigger>}
        </TabsList>

        <TabsContent value="source-import">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><Upload className="w-5 h-5" />导入新版绩效工作簿</CardTitle>
              <p className="text-sm text-muted-foreground">先校验实际上传的 Excel，再查看诊断。保留原权重，维度内合计异常仅告警；综合权重须合计 100%，存在阻断问题时不能发布。</p>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="flex flex-col gap-3 rounded-md border border-dashed p-4 sm:flex-row sm:items-end sm:justify-between">
                <div className="space-y-2">
                  <Label htmlFor="source-workbook">来源工作簿（.xlsx）</Label>
                  <Input
                    id="source-workbook"
                    type="file"
                    disabled={sourceValidating || !!sourceImporting || sourceLoadingDiagnostics || sourceLoadingHRDiagnostics}
                    accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                    onChange={event => {
                      setSourceFile(event.target.files?.[0] || null)
                      clearSourceResult()
                    }}
                  />
                  {sourceFile && <p className="text-xs text-muted-foreground">已选择：{sourceFile.name}（{Math.ceil(sourceFile.size / 1024)} KB）</p>}
                </div>
                <Button onClick={validateSourceWorkbook} disabled={!sourceFile || sourceValidating || !!sourceImporting || sourceLoadingDiagnostics || sourceLoadingHRDiagnostics}>
                  {sourceValidating ? <><RefreshCw className="mr-2 h-4 w-4 animate-spin" />校验中...</> : <><FileSearch className="mr-2 h-4 w-4" />开始校验</>}
                </Button>
              </div>

              {sourcePreview && (
                <div className="space-y-4">
                  <div className="rounded-md border bg-muted/20 p-4">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <h3 className="font-medium">来源版本：{sourcePreview.parser_version}</h3>
                        <p className="mt-1 break-all text-xs text-muted-foreground">SHA-256：{sourcePreview.source_sha256}</p>
                      </div>
                      <Badge variant={sourceBlockingCount ? "destructive" : sourcePreview.can_publish ? "default" : "outline"}>
                        {sourceImportResult ? "已处理" : sourceBlockingCount ? `${sourceBlockingCount} 个阻断问题` : sourcePreview.can_publish ? "可发布" : "可保存草稿"}
                      </Badge>
                    </div>
                    <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
                      {[
                        ["工作表", sourcePreview.sheet_count],
                        ["岗位模板", sourcePreview.summary.position_templates],
                        ["指标", sourcePreview.summary.indicators],
                        ["源部门", sourcePreview.summary.source_departments],
                        ["HR汇总表", sourcePreview.summary.hr_summary_sheets],
                      ].map(([label, value]) => <div key={String(label)} className="rounded border bg-background px-3 py-2 text-sm"><div className="text-muted-foreground">{label}</div><div className="text-lg font-semibold">{value}</div></div>)}
                    </div>
                  </div>

                  <div className="flex flex-wrap gap-2">
                    <Button variant="outline" onClick={loadSourceDiagnostics} disabled={sourceLoadingDiagnostics || !!sourceImporting}>
                      {sourceLoadingDiagnostics ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <FileSearch className="mr-2 h-4 w-4" />}完整来源诊断
                    </Button>
                    <Button variant="outline" onClick={loadSourceHRDiagnostics} disabled={sourceLoadingHRDiagnostics || !!sourceImporting}>
                      {sourceLoadingHRDiagnostics ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <FileSearch className="mr-2 h-4 w-4" />}HR汇总诊断
                    </Button>
                    <Button variant="secondary" onClick={() => importSourceWorkbook("draft")} disabled={!sourcePreview.can_import_draft || !!sourceImporting || !!sourceImportResult || sourceLoadingDiagnostics || sourceLoadingHRDiagnostics}>
                      {sourceImporting === "draft" ? <><RefreshCw className="mr-2 h-4 w-4 animate-spin" />导入中...</> : "保存为草稿"}
                    </Button>
                    <Button onClick={() => importSourceWorkbook("publish")} disabled={sourceBlockingCount > 0 || !sourcePreview.can_publish || !!sourceImporting || !!sourceImportResult || sourceLoadingDiagnostics || sourceLoadingHRDiagnostics}>
                      {sourceImporting === "publish" ? <><RefreshCw className="mr-2 h-4 w-4 animate-spin" />发布中...</> : "导入并发布"}
                    </Button>
                  </div>

                  {sourceImportResult && <div className="space-y-2 rounded border border-green-300 bg-green-50 p-3 text-sm text-green-900" role="status">
                    <p>导入完成：新增 {sourceImportResult.created_templates} 套，复用 {sourceImportResult.reused_templates} 套模板。复用数量不表示已有模板被重新创建或覆盖，请在列表查看实际版本和状态。</p>
                    <Button variant="outline" size="sm" onClick={() => setTemplateTabValue("templates")}>查看模板列表</Button>
                    <p className="text-xs">如需再次操作，点击“开始校验”刷新来源状态；修订后的 Excel 请重新选择文件。</p>
                  </div>}

                  <div className="space-y-2">
                    <h3 className="font-medium">来源诊断（{sourceDiagnostics.length}）：{sourceBlockingCount} 项阻断，{sourceWarningCount} 项告警</h3>
                    {sourceDiagnostics.length === 0 ? <div className="rounded border border-dashed p-4 text-sm text-muted-foreground">暂无诊断；点击“完整来源诊断”查看工作表、单元格和修复建议。</div> : (
                      <div className="overflow-x-auto rounded border">
                        <Table>
                          <TableHeader><TableRow><TableHead>级别</TableHead><TableHead>代码</TableHead><TableHead>来源</TableHead><TableHead>说明</TableHead><TableHead>修复建议</TableHead></TableRow></TableHeader>
                          <TableBody>{sourceDiagnostics.map((diagnostic, index) => <TableRow key={`${diagnostic.code}-${diagnostic.sheet}-${diagnostic.cell}-${index}`}>
                            <TableCell><Badge variant={diagnostic.severity === "blocking" || diagnostic.severity === "error" ? "destructive" : diagnostic.severity === "warning" ? "outline" : "secondary"}>{diagnostic.severity}</Badge></TableCell>
                            <TableCell className="font-mono text-xs">{diagnostic.code}</TableCell>
                            <TableCell className="whitespace-nowrap text-xs">{diagnostic.sheet || "-"}{diagnostic.cell ? `!${diagnostic.cell}` : ""}</TableCell>
                            <TableCell>{diagnostic.message}</TableCell>
                            <TableCell className="text-muted-foreground">{diagnostic.repair_hint || "-"}</TableCell>
                          </TableRow>)}</TableBody>
                        </Table>
                      </div>
                    )}
                  </div>

                  {sourceHRDiagnostics.length > 0 && <div className="space-y-2">
                    <h3 className="font-medium">HR 汇总公式诊断（{sourceHRDiagnostics.length}）</h3>
                    <div className="overflow-x-auto rounded border">
                      <Table><TableHeader><TableRow><TableHead>级别</TableHead><TableHead>周期</TableHead><TableHead>来源</TableHead><TableHead>公式</TableHead><TableHead>说明</TableHead></TableRow></TableHeader>
                        <TableBody>{sourceHRDiagnostics.map((diagnostic, index) => <TableRow key={`${diagnostic.code}-${diagnostic.sheet}-${diagnostic.cell}-${index}`}>
                          <TableCell><Badge variant={diagnostic.severity === "blocking" || diagnostic.severity === "error" ? "destructive" : "outline"}>{diagnostic.severity}</Badge></TableCell>
                          <TableCell>{diagnostic.period_type || "-"}</TableCell>
                          <TableCell className="whitespace-nowrap text-xs">{diagnostic.sheet || "-"}{diagnostic.cell ? `!${diagnostic.cell}` : ""}</TableCell>
                          <TableCell className="max-w-72 font-mono text-xs">{diagnostic.expanded_formula || diagnostic.formula || "-"}</TableCell>
                          <TableCell>{diagnostic.message}</TableCell>
                        </TableRow>)}</TableBody>
                      </Table>
                    </div>
                  </div>}

                  {!sourceDiagnostics.some(diagnostic => diagnostic.severity === "blocking" || diagnostic.severity === "error") && sourcePreview.can_publish && <div className="flex items-center gap-2 rounded border border-green-300 bg-green-50 p-3 text-sm text-green-800"><CheckCircle2 className="h-4 w-4" />未发现阻断诊断，可以发布。</div>}
                  {sourceDiagnostics.some(diagnostic => diagnostic.severity === "blocking" || diagnostic.severity === "error") && <div className="flex items-center gap-2 rounded border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900"><AlertTriangle className="h-4 w-4" />请先修正阻断项；当前只允许保存草稿，重新校验后再发布。</div>}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="templates">
          <Card>
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <CardTitle className="flex items-center"><ClipboardList className="w-5 h-5 mr-2" />模板列表</CardTitle>
                <div className="flex flex-wrap gap-2">
                  <Select value={filterDepartmentId ? String(filterDepartmentId) : "all"} onValueChange={value => setFilterDepartmentId(value === "all" ? undefined : Number(value))}>
                    <SelectTrigger className="w-[160px]"><SelectValue placeholder="全部部门" /></SelectTrigger>
                    <SelectContent><SelectItem value="all">全部部门</SelectItem>{departments.map(department => <SelectItem key={department.id} value={String(department.id)}>{department.name}</SelectItem>)}</SelectContent>
                  </Select>
                  <Input className="w-[180px]" placeholder="岗位类型" value={filterPositionType} onChange={event => setFilterPositionType(event.target.value)} />
                  <Select value={filterPeriod || "all"} onValueChange={value => setFilterPeriod(value === "all" ? "" : value)}>
                    <SelectTrigger className="w-[120px]"><SelectValue placeholder="全部周期" /></SelectTrigger>
                    <SelectContent><SelectItem value="all">全部周期</SelectItem><SelectItem value="monthly">月度</SelectItem><SelectItem value="quarterly">季度</SelectItem><SelectItem value="yearly">年度</SelectItem></SelectContent>
                  </Select>
                  <Button variant="outline" size="sm" onClick={() => setShowHistoricalTemplates(value => !value)}>
                    {showHistoricalTemplates ? "隐藏历史版本" : "显示历史版本"}
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {loading ? (
                <LoadingInline className="py-8" message="加载中..." />
              ) : visibleTemplates.length === 0 ? (
                <div className="text-center py-8 text-gray-500">
                  暂无当前模板；可点击“显示历史版本”查看和清理已停用模板。
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>模板名称</TableHead>
                      <TableHead>适用范围</TableHead>
                      <TableHead>描述</TableHead>
                      <TableHead>考核周期</TableHead>
                      <TableHead>版本/状态</TableHead>
                      <TableHead>创建时间</TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {visibleTemplates.map(template => (
                      <TableRow key={template.id}>
                        <TableCell className="font-medium">{templateDisplayName(template.name)}</TableCell>
                        <TableCell>{departments.find(department => department.id === template.department_id)?.name || "通用"} / {template.position_type || "未指定岗位"}</TableCell>
                        <TableCell>{template.description}</TableCell>
                        <TableCell>
                          <Badge variant="outline">{getPeriodLabel(template.period)}</Badge>
                        </TableCell>
                        <TableCell><div className="flex items-center gap-2"><Badge variant="outline">V{template.version || 1}</Badge><Badge variant={template.status === "published" ? "default" : template.status === "retired" ? "secondary" : "outline"}>{template.status === "published" ? "已发布" : template.status === "retired" ? "已停用" : "草稿"}</Badge></div></TableCell>
                        <TableCell>{new Date(template.created_at).toLocaleDateString()}</TableCell>
                        <TableCell className="text-right space-x-2">
                          <Button variant="outline" size="sm" onClick={() => handleSelectTemplate(template)}>
                            <Eye className="w-4 h-4" />
                          </Button>
                          <Button variant="outline" size="sm" onClick={() => handleEditTemplate(template)}>
                            <Edit className="w-4 h-4" />
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => handleDeleteTemplate(template.id)}
                            disabled={template.status === "published"}
                            title={template.status === "published" ? "请先停用当前版本，再清理未引用的历史版本" : "删除未引用版本"}
                            aria-label={template.status === "published" ? "请先停用当前版本，再清理未引用的历史版本" : "删除未引用版本"}
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="items">
          {selectedTemplate && (
            <Card>
              <CardHeader>
                <CardTitle className="flex flex-wrap items-center justify-between gap-4">
                  <div className="flex items-center">
                    <Settings className="w-5 h-5 mr-2" />
                    KPI项目配置
                  </div>
                  <Button
                    onClick={() => {
                      setEditingItem(null)
                      setItemFormData(defaultItemFormData)
                      setItemDialogOpen(true)
                    }}
                  >
                    <Plus className="w-4 h-4 mr-2" />
                    添加项目
                  </Button>
                  <Dialog open={itemDialogOpen} onOpenChange={setItemDialogOpen}>
                  <DialogContent className="w-[95vw] sm:max-w-md mx-auto">
                    <DialogHeader>
                      <DialogTitle>{editingItem ? "编辑KPI项目" : "添加KPI项目"}</DialogTitle>
                    </DialogHeader>
                    <DialogBody>
                      <form id="template-item-form" onSubmit={handleItemSubmit} className="space-y-4">
                        <div className="flex flex-col gap-2">
                          <Label htmlFor="item-name">项目名称</Label>
                          <Input
                            id="item-name"
                            value={itemFormData.name}
                            onChange={e => setItemFormData({ ...itemFormData, name: e.target.value })}
                            required
                          />
                        </div>
                        <div className="flex flex-col gap-2">
                          <Label htmlFor="item-description">项目描述</Label>
                          <Textarea
                            id="item-description"
                            value={itemFormData.description}
                            className="max-h-80"
                            maxLength={800}
                            onChange={e => setItemFormData({ ...itemFormData, description: e.target.value })}
                          />
                        </div>
                        <div className="flex flex-col gap-2">
                          <Label htmlFor="max-score">满分</Label>
                          <Input
                            id="max-score"
                            type="number"
                            value={itemFormData.max_score}
                            onChange={e => e.target.value !== "" && setItemFormData({ ...itemFormData, max_score: parseInt(e.target.value) })}
                            required
                          />
                          <div className="text-sm text-muted-foreground">
                            <p>1、如果满分小于0，则输入应在「满分-0」之间；</p>
                            <p>2、如果满分大于0，则输入应在「0-满分」之间；</p>
                            <p>3、如果满分等于0，则可输入任意分数。</p>
                          </div>
                        </div>
                        <div className="flex flex-col gap-2">
                          <Label htmlFor="order">排序</Label>
                          <Input
                            id="order"
                            type="number"
                            value={itemFormData.order}
                            onChange={e => e.target.value !== "" && setItemFormData({ ...itemFormData, order: parseInt(e.target.value) })}
                            required
                          />
                        </div>
                      </form>
                    </DialogBody>
                    <DialogFooter className="flex-col-reverse sm:flex-row sm:justify-end gap-2 sm:space-x-2 sm:gap-0">
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => setItemDialogOpen(false)}
                        className="w-full sm:w-auto"
                      >
                        取消
                      </Button>
                      <Button type="submit" form="template-item-form" className="w-full sm:w-auto">
                        {editingItem ? "更新" : "创建"}
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="mb-4 p-4 bg-muted/50 rounded-lg">
                  <h3 className="text-sm font-medium mb-2 text-foreground">模板说明</h3>
                  <ul className="text-sm space-y-1 text-muted-foreground">
                    <li>• 当前模板: <span className="font-semibold">{selectedTemplate.name}</span></li>
                    <li>• 总项目数: <span className="font-semibold">{items.length}</span></li>
                    <li>• 总分: <span className="font-semibold">{formatScore(templateScoreMaximum(selectedTemplate, items))}分</span></li>
                  </ul>
                </div>
                <div className="mb-4 rounded-lg border bg-background p-4 text-sm">
                  <h3 className="mb-2 font-medium text-foreground">来源版本与诊断</h3>
                  <div className="grid gap-1 text-muted-foreground md:grid-cols-2">
                    <div>来源版本：{selectedTemplate.source_catalog_version || "手工维护"}</div>
                    <div>来源工作表：{selectedTemplate.source_sheet || "-"}</div>
                    <div className="md:col-span-2 break-all">SHA-256：{selectedTemplate.source_workbook_sha256 || "-"}</div>
                    <div>来源诊断：{selectedTemplate.diagnostic_status || (selectedTemplate.source_blocked ? "blocking" : "ok")}</div>
                    <div>诊断条目：{selectedTemplate.source_issues?.length || 0}</div>
                  </div>
                </div>
                {items.length === 0 ? (
                  <div className="text-center py-8 text-gray-500">暂无KPI项目，请点击&quot;添加项目&quot;创建</div>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>排序</TableHead>
                        <TableHead>项目名称</TableHead>
                        <TableHead>描述</TableHead>
                        <TableHead>满分</TableHead>
                        <TableHead className="text-right">操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {items.map(item => (
                        <TableRow key={item.id}>
                          <TableCell>
                            <Badge variant="outline">{item.order}</Badge>
                          </TableCell>
                          <TableCell className="font-medium">{item.name}</TableCell>
                          <TableCell className="min-w-40">
                            <pre className="whitespace-pre-wrap break-words text-sm text-muted-foreground line-clamp-5">
                              {item.description}
                            </pre>
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary">{formatScore(item.max_score)}分</Badge>
                          </TableCell>
                          <TableCell className="text-right space-x-2">
                            <Button variant="outline" size="sm" onClick={() => handleEditItem(item)}>
                              <Edit className="w-4 h-4" />
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => handleDeleteItem(item.id)}>
                              <Trash2 className="w-4 h-4" />
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
          )}
        </TabsContent>

        {selectedTemplate && <TabsContent value="calculation-structure">
          <TemplateCalculationEditor
            template={selectedTemplate}
            onChanged={template => {
              setSelectedTemplate(template)
              setExportLayout(template.export_layout || exportPresets.final_signoff)
              setTemplates(current => current.some(item => item.id === template.id)
                ? current.map(item => item.id === template.id ? template : item)
                : [...current, template])
              fetchTemplateItems(template.id)
            }}
          />
        </TabsContent>}

        <TabsContent value="export-layout">
          {selectedTemplate && (
            <Card>
              <CardHeader>
                <CardTitle className="flex flex-wrap items-center justify-between gap-3">
                  <span className="flex items-center"><Settings className="w-5 h-5 mr-2" />结果导出版式</span>
                  <div className="flex flex-wrap gap-2">
                    {exportLayoutLocked && (
                      <Button variant="outline" onClick={cloneSelectedTemplate} disabled={cloningTemplate}>
                        <Copy className="w-4 h-4 mr-2" />{cloningTemplate ? "复制中..." : "复制新版本后编辑"}
                      </Button>
                    )}
                    <Button onClick={saveExportLayout} disabled={savingExportLayout || exportLayoutLocked}>
                      <Save className="w-4 h-4 mr-2" />{savingExportLayout ? "保存中..." : "保存版式"}
                    </Button>
                  </div>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-6">
                {exportLayoutLocked && (
                  <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
                    当前模板已{selectedTemplate.status === "published" ? "发布" : "停用"}，历史考核必须保持原版式。请先复制新版本，再修改和保存导出版式。
                  </div>
                )}
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>系统预设</Label>
                    <Select
                      disabled={exportLayoutLocked}
                      value={exportLayout.preset}
                      onValueChange={(value: ResultExportLayout["preset"]) => setExportLayout({ ...exportPresets[value] })}
                    >
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="final_signoff">最终签字简版</SelectItem>
                        <SelectItem value="full_process">评分过程完整版</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="export-title">表格标题</Label>
                    <Input disabled={exportLayoutLocked} id="export-title" maxLength={60} value={exportLayout.title} onChange={event => setExportLayout(current => ({ ...current, title: event.target.value }))} />
                  </div>
                  <div className="space-y-2">
                    <Label>打印方向</Label>
                    <Select disabled={exportLayoutLocked} value={exportLayout.orientation} onValueChange={(value: ResultExportLayout["orientation"]) => setExportLayout(current => ({ ...current, orientation: value }))}>
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent><SelectItem value="portrait">竖向（A4）</SelectItem><SelectItem value="landscape">横向（A4）</SelectItem></SelectContent>
                    </Select>
                    <p className="text-xs text-muted-foreground">只影响之后发布的新版本；已经创建的考核继续使用自身快照。</p>
                  </div>
                </div>

                <div className="space-y-3">
                  <div>
                    <h3 className="font-medium">评分明细列</h3>
                    <p className="text-sm text-muted-foreground">勾选字段后，可在下方调整实际导出顺序。指标和最终得分为必选。</p>
                  </div>
                  <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                    {exportFields.map(field => (
                      <label key={field.key} className="flex items-center gap-2 rounded-md border p-3 text-sm">
                        <input
                          type="checkbox"
                          checked={exportLayout.columns.includes(field.key)}
                          disabled={field.required || exportLayoutLocked}
                          onChange={event => toggleExportColumn(field.key, event.target.checked)}
                        />
                        {field.label}{field.required && <Badge variant="outline">必选</Badge>}
                      </label>
                    ))}
                  </div>
                  <div className="space-y-2">
                    {exportLayout.columns.map((column, index) => (
                      <div key={column} className="flex items-center justify-between rounded-md border px-3 py-2">
                        <span>{index + 1}. {exportFields.find(field => field.key === column)?.label || column}</span>
                        <div className="flex gap-1">
                          <Button type="button" variant="ghost" size="sm" disabled={exportLayoutLocked || index === 0} onClick={() => moveExportColumn(index, -1)}><ArrowUp className="w-4 h-4" /></Button>
                          <Button type="button" variant="ghost" size="sm" disabled={exportLayoutLocked || index === exportLayout.columns.length - 1} onClick={() => moveExportColumn(index, 1)}><ArrowDown className="w-4 h-4" /></Button>
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="rounded-md border border-dashed bg-muted/30 p-4">
                    <div className="text-sm font-medium">导出预览</div>
                    <div className={`mt-3 overflow-hidden border bg-background shadow-sm ${exportLayout.orientation === "portrait" ? "max-w-[420px]" : "w-full"}`}>
                      <div className="border-b p-3 text-center text-sm font-semibold">{exportLayout.title || "绩效考核结果确认表"}</div>
                      <div className="grid gap-1 p-3 text-xs" style={{ gridTemplateColumns: `repeat(${Math.min(exportLayout.columns.length, 4)}, minmax(0, 1fr))` }}>
                        {exportLayout.columns.map(column => <div key={column} className="truncate border p-2">{exportFields.find(field => field.key === column)?.label || column}</div>)}
                      </div>
                      <div className="border-t p-3 text-xs text-muted-foreground">{exportLayout.orientation === "portrait" ? "A4 竖向" : "A4 横向"} · 签字栏 {exportLayout.signature_labels.length} 个</div>
                    </div>
                  </div>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="flex items-center gap-2 rounded-md border p-3 text-sm">
                    <input disabled={exportLayoutLocked} type="checkbox" checked={exportLayout.show_summary} onChange={event => setExportLayout(current => ({ ...current, show_summary: event.target.checked }))} />显示总结评价
                  </label>
                  <label className="flex items-center gap-2 rounded-md border p-3 text-sm">
                    <input disabled={exportLayoutLocked} type="checkbox" checked={exportLayout.show_employee_opinion} onChange={event => setExportLayout(current => ({ ...current, show_employee_opinion: event.target.checked }))} />显示员工确认意见
                  </label>
                </div>

                <div className="space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <div><h3 className="font-medium">签字栏</h3><p className="text-sm text-muted-foreground">默认按模板评分职责生成；自定义后保留你的设置。支持 1-6 个签字栏。</p></div>
                    <Button
                      type="button"
                      variant="outline"
                      disabled={exportLayoutLocked || exportLayout.signature_labels.length >= 6}
                      onClick={() => setExportLayout(current => ({ ...current, signature_labels: [...current.signature_labels, `签字人${current.signature_labels.length + 1}`] }))}
                    ><Plus className="w-4 h-4 mr-1" />添加</Button>
                  </div>
                  {exportLayout.signature_labels.map((label, index) => (
                    <div key={index} className="flex gap-2">
                      <Input
                        disabled={exportLayoutLocked}
                        maxLength={20}
                        value={label}
                        onChange={event => setExportLayout(current => ({ ...current, signature_labels: current.signature_labels.map((item, itemIndex) => itemIndex === index ? event.target.value : item) }))}
                      />
                      <Button
                        type="button"
                        variant="outline"
                        size="icon"
                        disabled={exportLayoutLocked || exportLayout.signature_labels.length <= 1}
                        onClick={() => setExportLayout(current => ({ ...current, signature_labels: current.signature_labels.filter((_, itemIndex) => itemIndex !== index) }))}
                      ><X className="w-4 h-4" /></Button>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}
