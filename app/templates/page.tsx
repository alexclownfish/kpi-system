"use client"

import { useState, useEffect } from "react"
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
import { Plus, Edit, Trash2, ClipboardList, Settings, Eye, ArrowUp, ArrowDown, Save, X } from "lucide-react"
import { templateApi, itemApi, type KPITemplate, type KPIItem, type ResultExportColumn, type ResultExportLayout } from "@/lib/api"
import { useAppContext } from "@/lib/app-context"
import { getPeriodLabel, formatScore } from "@/lib/utils"
import { LoadingInline } from "@/components/loading"
import { toast } from "sonner"

const defaultTemplateFormData = {
  name: "",
  description: "",
  period: "monthly",
  is_active: true,
}

const defaultItemFormData = {
  name: "",
  description: "",
  max_score: 0,
  order: 1,
}

const exportFields: Array<{ key: ResultExportColumn; label: string; required?: boolean }> = [
  { key: "item_name", label: "指标", required: true },
  { key: "item_description", label: "指标说明" },
  { key: "max_score", label: "满分" },
  { key: "self_score", label: "自评分" },
  { key: "self_comment", label: "自评说明" },
  { key: "manager_score", label: "主管评分" },
  { key: "manager_comment", label: "主管说明" },
  { key: "hr_score", label: "HR评分" },
  { key: "hr_comment", label: "HR说明" },
  { key: "final_score", label: "最终得分", required: true },
  { key: "final_comment", label: "最终评价" },
]

const exportPresets: Record<ResultExportLayout["preset"], ResultExportLayout> = {
  final_signoff: {
    version: 1,
    preset: "final_signoff",
    title: "绩效考核结果确认表",
    columns: ["item_name", "max_score", "final_score", "final_comment"],
    show_summary: true,
    show_employee_opinion: true,
    signature_labels: ["员工签字", "直属主管签字", "HR签字", "签字日期"],
  },
  full_process: {
    version: 1,
    preset: "full_process",
    title: "绩效考核结果确认表",
    columns: exportFields.map(field => field.key),
    show_summary: true,
    show_employee_opinion: true,
    signature_labels: ["员工签字", "直属主管签字", "HR签字", "签字日期"],
  },
}

export default function TemplatesPage() {
  const { Alert, Confirm } = useAppContext()
  const [templates, setTemplates] = useState<KPITemplate[]>([])
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

  // 获取模板列表
  const fetchTemplates = async () => {
    try {
      const response = await templateApi.getAll()
      setTemplates(response.data || [])
    } catch (error) {
      console.error("获取模板列表失败:", error)
    }
    setLoading(false)
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

  useEffect(() => {
    fetchTemplates()
  }, [])

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
    if (!selectedTemplate) return
    try {
      setSavingExportLayout(true)
      const response = await templateApi.update(selectedTemplate.id, { export_layout: exportLayout })
      setSelectedTemplate(response.data)
      setExportLayout(response.data.export_layout)
      setTemplates(current => current.map(template => template.id === response.data.id ? response.data : template))
      toast.success("结果导出版式已保存")
    } catch (error) {
      const detail = (error as { response?: { data?: { message?: string; error?: string } } }).response?.data
      await Alert("保存失败", detail?.message || detail?.error || "结果导出版式保存失败，请检查配置后重试。")
    } finally {
      setSavingExportLayout(false)
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
            <Button className="w-full sm:w-auto lg:mt-8">
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
          {selectedTemplate && (
            <TabsTrigger value="items">KPI项目 {selectedTemplate && `(${selectedTemplate.name})`}</TabsTrigger>
          )}
          {selectedTemplate && <TabsTrigger value="export-layout">结果导出设置</TabsTrigger>}
        </TabsList>

        <TabsContent value="templates">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center">
                <ClipboardList className="w-5 h-5 mr-2" />
                模板列表
              </CardTitle>
            </CardHeader>
            <CardContent>
              {loading ? (
                <LoadingInline className="py-8" message="加载中..." />
              ) : templates.length === 0 ? (
                <div className="text-center py-8 text-gray-500">暂无模板数据</div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>模板名称</TableHead>
                      <TableHead>描述</TableHead>
                      <TableHead>考核周期</TableHead>
                      {/* <TableHead>状态</TableHead> */}
                      <TableHead>创建时间</TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {templates.map(template => (
                      <TableRow key={template.id}>
                        <TableCell className="font-medium">{template.name}</TableCell>
                        <TableCell>{template.description}</TableCell>
                        <TableCell>
                          <Badge variant="outline">{getPeriodLabel(template.period)}</Badge>
                        </TableCell>
                        {/* <TableCell>
                          <Badge variant={template.is_active ? "default" : "secondary"}>
                            {template.is_active ? "活跃" : "停用"}
                          </Badge>
                        </TableCell> */}
                        <TableCell>{new Date(template.created_at).toLocaleDateString()}</TableCell>
                        <TableCell className="text-right space-x-2">
                          <Button variant="outline" size="sm" onClick={() => handleSelectTemplate(template)}>
                            <Eye className="w-4 h-4" />
                          </Button>
                          <Button variant="outline" size="sm" onClick={() => handleEditTemplate(template)}>
                            <Edit className="w-4 h-4" />
                          </Button>
                          <Button variant="outline" size="sm" onClick={() => handleDeleteTemplate(template.id)}>
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
                    <li>• 总分: <span className="font-semibold">{formatScore(items.reduce((sum, item) => sum + item.max_score, 0))}分</span></li>
                  </ul>
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

        <TabsContent value="export-layout">
          {selectedTemplate && (
            <Card>
              <CardHeader>
                <CardTitle className="flex flex-wrap items-center justify-between gap-3">
                  <span className="flex items-center"><Settings className="w-5 h-5 mr-2" />结果导出版式</span>
                  <Button onClick={saveExportLayout} disabled={savingExportLayout}>
                    <Save className="w-4 h-4 mr-2" />{savingExportLayout ? "保存中..." : "保存版式"}
                  </Button>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>系统预设</Label>
                    <Select
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
                    <Input id="export-title" maxLength={60} value={exportLayout.title} onChange={event => setExportLayout(current => ({ ...current, title: event.target.value }))} />
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
                          disabled={field.required}
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
                          <Button type="button" variant="ghost" size="sm" disabled={index === 0} onClick={() => moveExportColumn(index, -1)}><ArrowUp className="w-4 h-4" /></Button>
                          <Button type="button" variant="ghost" size="sm" disabled={index === exportLayout.columns.length - 1} onClick={() => moveExportColumn(index, 1)}><ArrowDown className="w-4 h-4" /></Button>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="flex items-center gap-2 rounded-md border p-3 text-sm">
                    <input type="checkbox" checked={exportLayout.show_summary} onChange={event => setExportLayout(current => ({ ...current, show_summary: event.target.checked }))} />显示总结评价
                  </label>
                  <label className="flex items-center gap-2 rounded-md border p-3 text-sm">
                    <input type="checkbox" checked={exportLayout.show_employee_opinion} onChange={event => setExportLayout(current => ({ ...current, show_employee_opinion: event.target.checked }))} />显示员工确认意见
                  </label>
                </div>

                <div className="space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <div><h3 className="font-medium">签字栏</h3><p className="text-sm text-muted-foreground">支持 1-6 个签字栏。</p></div>
                    <Button
                      type="button"
                      variant="outline"
                      disabled={exportLayout.signature_labels.length >= 6}
                      onClick={() => setExportLayout(current => ({ ...current, signature_labels: [...current.signature_labels, `签字人${current.signature_labels.length + 1}`] }))}
                    ><Plus className="w-4 h-4 mr-1" />添加</Button>
                  </div>
                  {exportLayout.signature_labels.map((label, index) => (
                    <div key={index} className="flex gap-2">
                      <Input
                        maxLength={20}
                        value={label}
                        onChange={event => setExportLayout(current => ({ ...current, signature_labels: current.signature_labels.map((item, itemIndex) => itemIndex === index ? event.target.value : item) }))}
                      />
                      <Button
                        type="button"
                        variant="outline"
                        size="icon"
                        disabled={exportLayout.signature_labels.length <= 1}
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
