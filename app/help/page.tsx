"use client"

import { useEffect, useRef, useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Users, FileText, Settings, Shield, MessageCircle, BookOpen, Building } from "lucide-react"
import { useAuth } from "@/lib/auth-context"
import { useRouter } from "next/navigation"
import { scrollToElement } from "@/lib/utils"

type HelpSection = "overview" | "roles" | "evaluation" | "hr-features" | "settings" | "faq"

export default function HelpPage() {
  const router = useRouter()
  const { isHR } = useAuth()
  const [activeSection, setActiveSection] = useState<HelpSection>("overview")
  const contentRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    // 只有当内容头部不可见时才滚动
    if (contentRef.current) {
      const rect = contentRef.current.getBoundingClientRect()
      const isTopVisible = rect.top >= 0 && rect.top < window.innerHeight
      if (!isTopVisible) {
        scrollToElement(contentRef.current, 24)
      }
    }
  }, [activeSection])

  // 帮助主题配置
  const helpSections = [
    {
      id: "overview" as HelpSection,
      label: "系统概览",
      icon: <BookOpen className="w-4 h-4" />,
      available: true,
    },
    {
      id: "roles" as HelpSection,
      label: "角色权限",
      icon: <Shield className="w-4 h-4" />,
      available: true,
    },
    {
      id: "evaluation" as HelpSection,
      label: "考核管理",
      icon: <FileText className="w-4 h-4" />,
      available: true,
    },
    {
      id: "hr-features" as HelpSection,
      label: "HR功能",
      icon: <Users className="w-4 h-4" />,
      available: isHR,
    },
    {
      id: "settings" as HelpSection,
      label: "系统设置",
      icon: <Settings className="w-4 h-4" />,
      available: true,
    },
    {
      id: "faq" as HelpSection,
      label: "常见问题",
      icon: <MessageCircle className="w-4 h-4" />,
      available: true,
    },
  ]

  // 渲染系统概览内容
  const renderOverviewContent = () => (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center">
          <BookOpen className="w-5 h-5 mr-2" />
          系统概览
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <h3 className="text-lg font-semibold mb-3">欢迎使用 KPI 考核系统</h3>
          <p className="text-muted-foreground mb-4">
            这是一个专为企业绩效考核设计的综合管理平台，支持多角色协作，让绩效管理变得更加高效和透明。
          </p>
        </div>

        <div>
          <h4 className="font-medium mb-2">🎯 系统主要功能</h4>
          <ul className="space-y-2 text-sm text-muted-foreground">
            <li>• <strong>考核管理</strong>：创建、填写和管理绩效考核</li>
            <li>• <strong>邀请评分</strong>：邀请同事参与评分，获得多角度评价</li>
            <li>• <strong>部门管理</strong>：组织架构和部门信息管理</li>
            <li>• <strong>员工管理</strong>：员工信息和权限管理</li>
            <li>• <strong>模板管理</strong>：KPI 考核模板的创建和维护</li>
            <li>• <strong>统计分析</strong>：考核数据的图表和报表分析</li>
            <li>• <strong>系统设置</strong>：个性化配置和系统管理</li>
          </ul>
        </div>

        <div>
          <h4 className="font-medium mb-2">🚀 快速入门</h4>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="p-3 bg-muted/50 rounded-lg">
              <h5 className="font-medium text-sm">员工用户</h5>
              <p className="text-xs text-muted-foreground mt-1">
                登录后可查看和填写分配给自己的考核任务
              </p>
            </div>
            <div className="p-3 bg-muted/50 rounded-lg">
              <h5 className="font-medium text-sm">主管用户</h5>
              <p className="text-xs text-muted-foreground mt-1">
                可以评估下属员工的考核并提供反馈
              </p>
            </div>
            <div className="p-3 bg-muted/50 rounded-lg">
              <h5 className="font-medium text-sm">HR 用户</h5>
              <p className="text-xs text-muted-foreground mt-1">
                拥有完整的系统管理权限，可管理所有功能模块并发起邀请评分
              </p>
            </div>
            <div className="p-3 bg-muted/50 rounded-lg">
              <h5 className="font-medium text-sm">被邀请评分人员</h5>
              <p className="text-xs text-muted-foreground mt-1">
                接收邀请对同事进行评分，提供多角度的评价参考
              </p>
            </div>
          </div>
        </div>

        {/* 快速导航 */}
        <div>
          <h4 className="font-medium mb-3">📚 快速导航</h4>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
            <Button 
              variant="outline" 
              size="sm" 
              className="justify-start h-auto p-3"
              onClick={() => setActiveSection("roles")}
            >
              <Shield className="w-4 h-4 mr-2" />
              <div className="text-left">
                <div className="font-medium text-sm">角色权限</div>
                <div className="text-xs text-muted-foreground">了解不同角色的功能</div>
              </div>
            </Button>
            <Button 
              variant="outline" 
              size="sm" 
              className="justify-start h-auto p-3"
              onClick={() => setActiveSection("evaluation")}
            >
              <FileText className="w-4 h-4 mr-2" />
              <div className="text-left">
                <div className="font-medium text-sm">考核管理</div>
                <div className="text-xs text-muted-foreground">学习如何使用考核功能</div>
              </div>
            </Button>
            {isHR && (
              <Button 
                variant="outline" 
                size="sm" 
                className="justify-start h-auto p-3"
                onClick={() => setActiveSection("hr-features")}
              >
                <Users className="w-4 h-4 mr-2" />
                <div className="text-left">
                  <div className="font-medium text-sm">HR功能</div>
                  <div className="text-xs text-muted-foreground">管理员功能指南</div>
                </div>
              </Button>
            )}
            <Button 
              variant="outline" 
              size="sm" 
              className="justify-start h-auto p-3"
              onClick={() => setActiveSection("settings")}
            >
              <Settings className="w-4 h-4 mr-2" />
              <div className="text-left">
                <div className="font-medium text-sm">系统设置</div>
                <div className="text-xs text-muted-foreground">个性化和配置</div>
              </div>
            </Button>
                          <Button 
                variant="outline" 
                size="sm" 
                className="justify-start h-auto p-3"
                onClick={() => router.push('/invitations')}
              >
                <Users className="w-4 h-4 mr-2" />
                <div className="text-left">
                  <div className="font-medium text-sm">邀请评分</div>
                  <div className="text-xs text-muted-foreground">查看邀请评分任务</div>
                </div>
              </Button>
              <Button 
                variant="outline" 
                size="sm" 
                className="justify-start h-auto p-3"
                onClick={() => setActiveSection("faq")}
              >
                <MessageCircle className="w-4 h-4 mr-2" />
                <div className="text-left">
                  <div className="font-medium text-sm">常见问题</div>
                  <div className="text-xs text-muted-foreground">快速解决问题</div>
                </div>
              </Button>
          </div>
        </div>

        <div>
          <h4 className="font-medium mb-2">💡 使用建议</h4>
          <ul className="space-y-1 text-sm text-muted-foreground">
            <li>• 首次使用建议先阅读&quot;角色权限&quot;了解功能范围</li>
            <li>• 遇到问题可查看&quot;常见问题&quot;或联系系统管理员</li>
            <li>• 定期检查考核任务，确保及时完成</li>
            <li>• 收到邀请评分时请及时响应，认真客观地进行评价</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )

  // 渲染角色权限内容
  const renderRolesContent = () => (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center">
          <Shield className="w-5 h-5 mr-2" />
          角色权限说明
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <p className="text-muted-foreground mb-6">
            系统根据不同角色分配相应的功能权限，确保数据安全和职责分明。
          </p>
        </div>

        {/* HR 角色 */}
        <div className="border rounded-lg p-4">
          <div className="flex items-center mb-3">
            <div className="w-8 h-8 bg-destructive/10 rounded-full flex items-center justify-center mr-3">
              <Shield className="w-4 h-4 text-destructive" />
            </div>
            <div>
              <h4 className="font-semibold">HR 用户</h4>
              <p className="text-sm text-muted-foreground">系统管理员，拥有最高权限</p>
            </div>
          </div>
          
          <div className="space-y-3">
            <div>
              <h5 className="font-medium text-sm mb-2">🏢 部门管理</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 创建、编辑和删除部门</li>
                <li>• 设置部门负责人</li>
                <li>• 查看部门组织架构</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">👥 员工管理</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 添加和编辑员工信息</li>
                <li>• 分配员工角色和权限</li>
                <li>• 管理员工部门归属</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">📋 KPI 模板</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 创建和编辑考核模板</li>
                <li>• 设置考核指标和权重</li>
                <li>• 启用或禁用模板</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">🤝 邀请评分</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 邀请其他人员参与员工评分</li>
                <li>• 管理和监控邀请评分进展</li>
                <li>• 查看邀请评分结果</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">📊 统计分析</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 查看全公司考核数据</li>
                <li>• 导出考核报表</li>
                <li>• 分析考核趋势</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 主管角色 */}
        <div className="border rounded-lg p-4">
          <div className="flex items-center mb-3">
            <div className="w-8 h-8 bg-primary/10 rounded-full flex items-center justify-center mr-3">
              <Users className="w-4 h-4 text-primary" />
            </div>
            <div>
              <h4 className="font-semibold">主管用户</h4>
              <p className="text-sm text-muted-foreground">部门负责人，管理下属考核</p>
            </div>
          </div>
          
          <div className="space-y-3">
            <div>
              <h5 className="font-medium text-sm mb-2">📝 考核管理</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 创建下属员工的考核任务</li>
                <li>• 评估下属的考核结果</li>
                <li>• 提供考核反馈和建议</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">👀 查看权限</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 查看本部门考核数据</li>
                <li>• 查看下属员工考核历史</li>
                <li>• 查看部门考核统计</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">⚙️ 系统设置</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 个人外观设置</li>
                <li>• 安全退出登录</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 员工角色 */}
        <div className="border rounded-lg p-4">
          <div className="flex items-center mb-3">
            <div className="w-8 h-8 bg-muted rounded-full flex items-center justify-center mr-3">
              <FileText className="w-4 h-4 text-muted-foreground" />
            </div>
            <div>
              <h4 className="font-semibold">员工用户</h4>
              <p className="text-sm text-muted-foreground">普通员工，参与考核流程</p>
            </div>
          </div>
          
          <div className="space-y-3">
            <div>
              <h5 className="font-medium text-sm mb-2">📋 考核任务</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 查看分配给自己的考核任务</li>
                <li>• 填写自评内容</li>
                <li>• 提交考核表单</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">🤝 邀请评分</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 接收并查看邀请评分通知</li>
                <li>• 接受或拒绝邀请评分</li>
                <li>• 对其他员工进行客观评分</li>
                <li>• 提交邀请评分结果</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">📈 查看权限</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 查看个人考核历史</li>
                <li>• 查看考核结果和反馈</li>
                <li>• 查看个人绩效趋势</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">⚙️ 系统设置</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 个人外观设置</li>
                <li>• 安全退出登录</li>
              </ul>
            </div>
          </div>
        </div>

        <div className="p-4 bg-muted/50 rounded-lg">
          <h4 className="font-medium mb-2">🔒 权限说明</h4>
          <ul className="text-sm text-muted-foreground space-y-1">
            <li>• 系统会根据登录用户的角色自动显示相应的功能菜单</li>
            <li>• 如需更改角色权限，请联系 HR 或系统管理员</li>
            <li>• 每个用户只能查看和操作自己权限范围内的数据</li>
            <li>• 所有操作都会记录日志，确保数据安全</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )

  // 渲染考核管理内容
  const renderEvaluationContent = () => (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center">
          <FileText className="w-5 h-5 mr-2" />
          考核管理指南
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <p className="text-muted-foreground mb-6">
            考核管理是系统的核心功能，支持完整的绩效考核流程管理。
          </p>
        </div>

        {/* 考核流程 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">📋 考核流程</h4>
          <div className="space-y-3">
            <div className="flex items-start space-x-3">
              <div className="w-6 h-6 bg-primary/10 rounded-full flex items-center justify-center text-sm font-medium text-primary">1</div>
              <div>
                <h5 className="font-medium text-sm">创建考核</h5>
                <p className="text-sm text-muted-foreground">HR 或主管创建考核任务，选择模板和被考核人员</p>
              </div>
            </div>
            <div className="flex items-start space-x-3">
              <div className="w-6 h-6 bg-primary/10 rounded-full flex items-center justify-center text-sm font-medium text-primary">2</div>
              <div>
                <h5 className="font-medium text-sm">员工自评</h5>
                <p className="text-sm text-muted-foreground">员工填写自我评价，对各项指标进行评分</p>
              </div>
            </div>
            <div className="flex items-start space-x-3">
              <div className="w-6 h-6 bg-primary/10 rounded-full flex items-center justify-center text-sm font-medium text-primary">3</div>
              <div>
                <h5 className="font-medium text-sm">主管评估</h5>
                <p className="text-sm text-muted-foreground">主管对员工进行评估，给出评分和反馈</p>
              </div>
            </div>
            <div className="flex items-start space-x-3">
              <div className="w-6 h-6 bg-primary/10 rounded-full flex items-center justify-center text-sm font-medium text-primary">4</div>
              <div>
                <h5 className="font-medium text-sm">邀请评分（可选）</h5>
                <p className="text-sm text-muted-foreground">HR可邀请其他人员对员工进行评分，获取多方面评价</p>
              </div>
            </div>
            <div className="flex items-start space-x-3">
              <div className="w-6 h-6 bg-primary/10 rounded-full flex items-center justify-center text-sm font-medium text-primary">5</div>
              <div>
                <h5 className="font-medium text-sm">HR审核</h5>
                <p className="text-sm text-muted-foreground">HR审核评估结果，确认最终评分</p>
              </div>
            </div>
            <div className="flex items-start space-x-3">
              <div className="w-6 h-6 bg-primary/10 rounded-full flex items-center justify-center text-sm font-medium text-primary">6</div>
              <div>
                <h5 className="font-medium text-sm">员工确认</h5>
                <p className="text-sm text-muted-foreground">员工确认最终得分，考核流程完成</p>
              </div>
            </div>
          </div>
        </div>

        {/* HR 操作指南 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">👔 HR 操作指南</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">创建考核</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 进入&quot;考核管理&quot;页面，点击&quot;创建考核&quot;按钮</li>
                <li>• 填写考核基本信息：考核周期（月度/季度/年度）、年份</li>
                <li>• 选择考核模板（从 KPI 模板中选择）</li>
                <li>• 选择被考核人员（可单选或批量选择）</li>
                <li>• 保存并发布考核任务</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">监控考核进度</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 在考核列表中查看各项考核的完成状态</li>
                <li>• 使用状态筛选查看不同阶段的考核</li>
                <li>• 查看考核统计数据和完成率</li>
                <li>• 在统计分析中导出考核数据和报表</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">邀请评分（可选）</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 在主管评估完成后点击&quot;邀请评分&quot;按钮</li>
                <li>• 选择要邀请的人员（可多选）</li>
                <li>• 输入邀请消息说明评分要求</li>
                <li>• 发送邀请并监控评分进展</li>
                <li>• 查看和管理邀请评分结果</li>
                <li>• 可以撤销未完成的邀请</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">HR审核</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 查看主管评估完成的考核</li>
                <li>• 审核评分的合理性和公平性</li>
                <li>• 必要时可以调整评分</li>
                <li>• 完成审核后提交给员工确认</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 主管操作指南 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">👨‍💼 主管操作指南</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">评估下属</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 在&quot;考核管理&quot;中查看需要评估的员工</li>
                <li>• 点击&quot;查看&quot;按钮进入考核详情页面</li>
                <li>• 查看员工的自评内容和评分</li>
                <li>• 对各项指标进行客观评分（可以与自评分不同）</li>
                <li>• 填写评估意见和改进建议</li>
                <li>• 完成所有项目评分后提交评估结果</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">管理权限</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 只能评估直接下属的员工考核</li>
                <li>• 不能评估自己的考核</li>
                <li>• 可以查看部门内的考核统计</li>
                <li>• 不能修改已提交的评估</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 员工操作指南 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">👤 员工操作指南</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">填写自评</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 在&quot;考核管理&quot;中查看分配给自己的考核任务</li>
                <li>• 点击&quot;查看&quot;按钮进入自评页面</li>
                <li>• 认真阅读各项 KPI 指标和要求</li>
                <li>• 如实填写工作成果和自我评分</li>
                <li>• 在自评说明中提供具体的工作案例和数据支撑</li>
                <li>• 完成所有项目评分后提交自评</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">查看结果</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 在考核历史中查看已完成的考核</li>
                <li>• 查看主管的评分和反馈意见</li>
                <li>• 确认最终得分（HR审核后）</li>
                <li>• 查看个人绩效表现趋势</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 被邀请人员操作指南 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">🤝 被邀请人员操作指南</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">接收邀请</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 在&quot;邀请评分&quot;页面查看收到的邀请</li>
                <li>• 阅读邀请消息了解评分要求</li>
                <li>• 查看被评估员工的基本信息</li>
                <li>• 根据自己对该员工的了解决定是否接受邀请</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">进行评分</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 点击&quot;接受邀请&quot;后进入评分界面</li>
                <li>• 根据对该员工的了解进行客观评分</li>
                <li>• 评分范围为0到各项目的最高分</li>
                <li>• 在评价说明中详细描述评分依据</li>
                <li>• 完成所有项目评分后点击&quot;完成评分&quot;</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">评分原则</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 基于客观事实和个人观察进行评分</li>
                <li>• 避免个人偏见，保持公正客观</li>
                <li>• 如果对某项指标不太了解，可以适当说明</li>
                <li>• 提供具体的改进建议和正面反馈</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 考核状态说明 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">📊 考核状态说明</h4>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div className="p-3 bg-blue-50 dark:bg-blue-950 rounded-lg">
              <div className="flex items-center mb-2">
                <div className="w-3 h-3 bg-blue-500 rounded-full mr-2"></div>
                <span className="font-medium text-sm">等待自评</span>
              </div>
              <p className="text-xs text-muted-foreground">员工需要填写自我评价</p>
            </div>
            <div className="p-3 bg-yellow-50 dark:bg-yellow-950 rounded-lg">
              <div className="flex items-center mb-2">
                <div className="w-3 h-3 bg-yellow-500 rounded-full mr-2"></div>
                <span className="font-medium text-sm">等待主管评估</span>
              </div>
              <p className="text-xs text-muted-foreground">主管需要评估员工考核</p>
            </div>
            <div className="p-3 bg-orange-50 dark:bg-orange-950 rounded-lg">
              <div className="flex items-center mb-2">
                <div className="w-3 h-3 bg-orange-500 rounded-full mr-2"></div>
                <span className="font-medium text-sm">等待HR审核</span>
              </div>
              <p className="text-xs text-muted-foreground">HR需要审核考核结果</p>
            </div>
            <div className="p-3 bg-purple-50 dark:bg-purple-950 rounded-lg">
              <div className="flex items-center mb-2">
                <div className="w-3 h-3 bg-purple-500 rounded-full mr-2"></div>
                <span className="font-medium text-sm">等待确认</span>
              </div>
              <p className="text-xs text-muted-foreground">员工需要确认最终得分</p>
            </div>
            <div className="p-3 bg-green-50 dark:bg-green-950 rounded-lg">
              <div className="flex items-center mb-2">
                <div className="w-3 h-3 bg-green-500 rounded-full mr-2"></div>
                <span className="font-medium text-sm">已完成</span>
              </div>
              <p className="text-xs text-muted-foreground">考核流程已全部完成</p>
            </div>
          </div>
          
          <div className="mt-4">
            <h5 className="font-medium text-sm mb-3">🤝 邀请评分状态</h5>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div className="p-3 bg-yellow-50 dark:bg-yellow-950 rounded-lg">
                <div className="flex items-center mb-2">
                  <div className="w-3 h-3 bg-yellow-500 rounded-full mr-2"></div>
                  <span className="font-medium text-sm">待接受</span>
                </div>
                <p className="text-xs text-muted-foreground">邀请已发送，等待对方响应</p>
              </div>
              <div className="p-3 bg-blue-50 dark:bg-blue-950 rounded-lg">
                <div className="flex items-center mb-2">
                  <div className="w-3 h-3 bg-blue-500 rounded-full mr-2"></div>
                  <span className="font-medium text-sm">进行中</span>
                </div>
                <p className="text-xs text-muted-foreground">邀请已接受，正在进行评分</p>
              </div>
              <div className="p-3 bg-green-50 dark:bg-green-950 rounded-lg">
                <div className="flex items-center mb-2">
                  <div className="w-3 h-3 bg-green-500 rounded-full mr-2"></div>
                  <span className="font-medium text-sm">已完成</span>
                </div>
                <p className="text-xs text-muted-foreground">评分已完成并提交</p>
              </div>
              <div className="p-3 bg-red-50 dark:bg-red-950 rounded-lg">
                <div className="flex items-center mb-2">
                  <div className="w-3 h-3 bg-red-500 rounded-full mr-2"></div>
                  <span className="font-medium text-sm">已拒绝</span>
                </div>
                <p className="text-xs text-muted-foreground">邀请被拒绝</p>
              </div>
            </div>
          </div>
        </div>

        <div className="p-4 bg-muted/50 rounded-lg">
          <h4 className="font-medium mb-2">💡 考核小贴士</h4>
          <ul className="text-sm text-muted-foreground space-y-1">
            <li>• 考核应该客观公正，基于具体的工作表现</li>
            <li>• 自评时要诚实，避免过高或过低估计</li>
            <li>• 主管评估时要提供具体的改进建议</li>
            <li>• 如果员工没有直接主管，自评后会直接进入HR审核</li>
            <li>• 邀请评分可以提供多角度的评价，帮助形成更全面的考核</li>
            <li>• 被邀请评分时要基于客观事实，避免个人偏见</li>
            <li>• 定期回顾考核结果，持续改进工作表现</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )

  // 渲染HR功能内容
  const renderHRFeaturesContent = () => (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center">
          <Users className="w-5 h-5 mr-2" />
          HR 功能指南
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <p className="text-muted-foreground mb-6">
            HR 用户拥有系统的完整管理权限，可以管理组织架构、员工信息、考核模板和数据分析。
          </p>
        </div>

        {/* 部门管理 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">🏢 部门管理</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">创建部门</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 点击&quot;创建部门&quot;按钮</li>
                <li>• 输入部门名称和描述</li>
                <li>• 选择上级部门（可选）</li>
                <li>• 设置部门负责人</li>
                <li>• 保存部门信息</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">管理部门</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 编辑部门信息：名称、描述、负责人</li>
                <li>• 查看部门员工列表</li>
                <li>• 调整部门层级结构</li>
                <li>• 删除空的部门</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 员工管理 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">👥 员工管理</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">添加员工</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 点击&quot;添加员工&quot;按钮</li>
                <li>• 填写员工基本信息：姓名、邮箱、职位</li>
                <li>• 选择员工所属部门</li>
                <li>• 设置员工角色：员工、主管、HR</li>
                <li>• 设置登录密码</li>
                <li>• 保存员工信息</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">管理员工</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 编辑员工信息：职位、部门、角色</li>
                <li>• 重置员工密码</li>
                <li>• 查看员工考核历史</li>
                <li>• 停用或启用员工账户</li>
                <li>• 员工部门调动</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">权限管理</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 员工：只能查看和填写自己的考核</li>
                <li>• 主管：可以评估下属员工的考核</li>
                <li>• HR：拥有所有功能的访问权限</li>
              </ul>
            </div>
          </div>
        </div>

        {/* KPI模板管理 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">📋 KPI 模板管理</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">创建模板</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 点击&quot;创建模板&quot;按钮</li>
                <li>• 填写模板名称和描述</li>
                <li>• 添加考核指标项目</li>
                <li>• 设置每个指标的权重</li>
                <li>• 定义评分标准和等级</li>
                <li>• 保存并启用模板</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">管理模板</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 编辑模板内容和权重</li>
                <li>• 复制模板创建新版本</li>
                <li>• 启用或停用模板</li>
                <li>• 查看模板使用记录</li>
                <li>• 删除未使用的模板</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">模板类型</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 通用模板：适用于大多数岗位</li>
                <li>• 专业模板：针对特定职位定制</li>
                <li>• 管理模板：适用于管理层考核</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 邀请评分管理 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">🤝 邀请评分管理</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">发起邀请</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 在考核详情页面，主管评估完成后可发起邀请</li>
                <li>• 选择需要邀请的人员（可多选）</li>
                <li>• 编写邀请消息，说明评分要求和背景</li>
                <li>• 发送邀请后系统会自动通知被邀请人</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">邀请管理</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 在邀请列表中查看所有邀请状态</li>
                <li>• 监控邀请的进展和完成情况</li>
                <li>• 对于未响应的邀请可以发送催促通知</li>
                <li>• 在必要时可以撤销未完成的邀请</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">评分查看</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 查看已完成的邀请评分结果</li>
                <li>• 分析不同评分人的评价差异</li>
                <li>• 将邀请评分作为HR审核的参考</li>
                <li>• 导出邀请评分数据进行分析</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">邀请状态</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 待接受：邀请已发送，等待对方响应</li>
                <li>• 进行中：邀请已接受，正在进行评分</li>
                <li>• 已完成：评分已完成并提交</li>
                <li>• 已拒绝：邀请被拒绝</li>
                <li>• 已撤销：邀请已被撤销</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 统计分析 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">📊 统计分析</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">数据看板</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 查看考核完成率统计</li>
                <li>• 查看员工绩效分布</li>
                <li>• 查看部门绩效排名</li>
                <li>• 查看考核趋势图表</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">报表导出</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 导出员工考核报表</li>
                <li>• 导出部门绩效报表</li>
                <li>• 导出考核统计数据</li>
                <li>• 自定义报表筛选条件</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">数据分析</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 分析绩效趋势变化</li>
                <li>• 识别高绩效员工</li>
                <li>• 发现需要改进的领域</li>
                <li>• 制定绩效改进计划</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 系统管理 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">⚙️ 系统管理</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">用户注册设置</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 开启或关闭用户注册功能</li>
                <li>• 设置默认新用户角色</li>
                <li>• 管理注册审批流程</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">数据备份</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 定期备份考核数据</li>
                <li>• 导出历史数据</li>
                <li>• 数据恢复操作</li>
              </ul>
            </div>
          </div>
        </div>

        <div className="p-4 bg-muted/50 rounded-lg">
          <h4 className="font-medium mb-2">👑 HR 管理建议</h4>
          <ul className="text-sm text-muted-foreground space-y-1">
            <li>• 定期检查和更新组织架构</li>
            <li>• 及时处理员工角色变更</li>
            <li>• 根据业务需求调整 KPI 模板</li>
            <li>• 定期分析绩效数据，优化考核流程</li>
            <li>• 合理使用邀请评分功能，获得多角度评价</li>
            <li>• 选择合适的人员进行邀请评分，确保评价质量</li>
            <li>• 保持系统数据的准确性和完整性</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )

  // 渲染系统设置内容
  const renderSettingsContent = () => (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center">
          <Settings className="w-5 h-5 mr-2" />
          系统设置指南
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <p className="text-muted-foreground mb-6">
            系统设置允许用户个性化界面和管理系统配置。
          </p>
        </div>

        {/* 快捷操作 */}
        <div className="border rounded-lg p-4 bg-muted/25">
          <h4 className="font-semibold mb-3">⚡ 快捷操作</h4>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push('/settings')}
              className="text-xs"
            >
              <Settings className="w-3 h-3 mr-1" />
              打开设置页面
            </Button>
                          <Button
                variant="outline"
                size="sm"
                onClick={() => router.push('/evaluations')}
                className="text-xs"
              >
                <FileText className="w-3 h-3 mr-1" />
                查看考核任务
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => router.push('/invitations')}
                className="text-xs"
              >
                <Users className="w-3 h-3 mr-1" />
                邀请评分
              </Button>
            {isHR && (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => router.push('/employees')}
                  className="text-xs"
                >
                  <Users className="w-3 h-3 mr-1" />
                  员工管理
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => router.push('/departments')}
                  className="text-xs"
                >
                  <Building className="w-3 h-3 mr-1" />
                  部门管理
                </Button>
              </>
            )}
          </div>
        </div>

        {/* 外观设置 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">🎨 外观设置</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">主题模式</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 浅色模式：适合明亮环境使用</li>
                <li>• 深色模式：适合暗光环境，减少眼疲劳</li>
                <li>• 跟随系统：自动根据设备系统主题切换</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">设置方法</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 进入&quot;系统设置&quot;页面</li>
                <li>• 在左侧菜单选择&quot;外观设置&quot;</li>
                <li>• 点击对应的主题卡片即可切换</li>
                <li>• 设置立即生效，无需保存</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 系统配置 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">⚙️ 系统配置（HR专用）</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">用户注册管理</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 开启注册：允许新用户自行注册</li>
                <li>• 关闭注册：仅允许管理员创建账户</li>
                <li>• 修改设置后需要点击&quot;保存设置&quot;按钮</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">安全设置</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 定期更改密码</li>
                <li>• 使用复杂密码</li>
                <li>• 及时退出系统</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 账户管理 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">👤 账户管理</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">退出登录</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 点击左侧菜单的&quot;退出登录&quot;</li>
                <li>• 确认退出操作</li>
                <li>• 系统会清除登录状态</li>
                <li>• 建议每次使用完毕后退出</li>
              </ul>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">密码安全</h5>
              <ul className="text-sm text-muted-foreground space-y-1 ml-4">
                <li>• 如需修改密码，请联系管理员</li>
                <li>• 不要与他人共享账户</li>
                <li>• 发现异常登录及时报告</li>
              </ul>
            </div>
          </div>
        </div>

        <div className="p-4 bg-muted/50 rounded-lg">
          <h4 className="font-medium mb-2">🔧 设置建议</h4>
          <ul className="text-sm text-muted-foreground space-y-1">
            <li>• 根据使用环境选择合适的主题模式</li>
            <li>• 定期检查系统设置是否符合需求</li>
            <li>• 及时查看邀请评分通知，避免错过重要任务</li>
            <li>• 保持良好的账户安全习惯</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )

  // 渲染常见问题内容
  const renderFAQContent = () => (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center">
          <MessageCircle className="w-5 h-5 mr-2" />
          常见问题解答
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <p className="text-muted-foreground mb-6">
            这里汇总了用户使用过程中最常遇到的问题和解决方案。
          </p>
        </div>

        {/* 登录问题 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">🔐 登录问题</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 忘记密码怎么办？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 请联系您的 HR 或系统管理员重置密码。出于安全考虑，系统不支持用户自行重置密码。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 登录后页面空白或报错？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 请尝试刷新页面或清除浏览器缓存。如果问题持续，请联系技术支持。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 账户被锁定怎么办？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 多次登录失败可能导致账户锁定。请联系管理员解锁账户。
              </p>
            </div>
          </div>
        </div>

        {/* 考核问题 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">📝 考核问题</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 看不到考核任务？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 检查是否有未分配给您的考核任务，或者考核可能尚未发布。请联系您的主管或 HR 确认。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 考核提交后能否修改？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 考核提交后通常不能修改。如需修改，请联系您的主管或 HR 处理。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 考核结果什么时候能看到？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 考核结果会在主管完成评估后自动显示。您可以在考核管理页面查看状态。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 如何查看历史考核记录？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 在考核管理页面，可以查看所有历史考核记录。使用筛选功能可以快速找到特定时间段的考核。
              </p>
            </div>
          </div>
        </div>

        {/* 权限问题 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">🔒 权限问题</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 无法访问某些功能？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 系统根据用户角色限制功能访问。如需更多权限，请联系 HR 调整您的角色权限。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 如何申请管理员权限？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 管理员权限需要由现有 HR 用户分配。请向您的 HR 部门提出申请。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 主管权限包括哪些功能？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 主管可以评估下属员工的考核、查看部门绩效数据，但不能管理员工信息和系统设置。
              </p>
            </div>
          </div>
        </div>

        {/* 技术问题 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">💻 技术问题</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 系统运行缓慢怎么办？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 请检查网络连接，关闭不必要的浏览器标签页，或尝试使用其他浏览器。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 支持哪些浏览器？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 推荐使用 Chrome、Firefox、Safari 或 Edge 的最新版本。不建议使用 IE 浏览器。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 数据丢失怎么办？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 系统会自动保存数据。如遇数据丢失，请立即联系技术支持进行数据恢复。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 如何导出数据？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 在统计分析页面，HR 用户可以导出各类报表。员工可以导出个人考核记录。
              </p>
            </div>
          </div>
        </div>

        {/* 邀请评分问题 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">🤝 邀请评分问题</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 为什么我收到了邀请评分？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: HR 认为您对被评估员工有一定了解，邀请您参与评分以获得更全面的评价。您可以选择接受或拒绝邀请。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 邀请评分是否匿名？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 邀请评分不是匿名的，您的评分和评价会显示评分人信息，但仅 HR 可以查看完整的评分详情。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 如果我对某个员工不够了解怎么办？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 如果您对该员工的工作表现不够了解，建议拒绝邀请。如果只是对某些指标不太了解，可以在评价说明中注明。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 邀请评分提交后能否修改？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 邀请评分提交后无法修改。请在提交前仔细检查所有评分和评价说明。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 邀请评分的结果会如何使用？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 邀请评分作为 HR 审核的重要参考，帮助形成更全面客观的评价，但不会直接影响最终的考核结果。
              </p>
            </div>
          </div>
        </div>

        {/* 操作问题 */}
        <div className="border rounded-lg p-4">
          <h4 className="font-semibold mb-3">⚙️ 操作问题</h4>
          <div className="space-y-4">
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 如何更改个人信息？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 个人信息需要由 HR 在员工管理页面进行修改。如需更改，请联系 HR 部门。
              </p>
            </div>
            
            <div>
              <h5 className="font-medium text-sm mb-2">Q: 系统支持移动端吗？</h5>
              <p className="text-sm text-muted-foreground ml-4">
                A: 系统采用响应式设计，支持在手机和平板上使用。建议使用最新版本的移动浏览器。
              </p>
            </div>
          </div>
        </div>

        <div className="p-4 bg-muted/50 rounded-lg">
          <h4 className="font-medium mb-2">🆘 获取帮助</h4>
          <ul className="text-sm text-muted-foreground space-y-1">
            <li>• 如果问题未在此解答，请联系您的主管或 HR</li>
            <li>• 邀请评分相关问题可直接联系发起邀请的 HR</li>
            <li>• 技术问题可联系系统管理员</li>
            <li>• 紧急情况请通过内部通讯工具联系相关负责人</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )

  // 渲染内容区域
  const renderContent = () => {
    switch (activeSection) {
      case "overview":
        return renderOverviewContent()
      case "roles":
        return renderRolesContent()
      case "evaluation":
        return renderEvaluationContent()
      case "hr-features":
        return isHR ? renderHRFeaturesContent() : (
          <div className="flex items-center justify-center h-64">
            <div className="text-center">
              <Shield className="w-12 h-12 mx-auto text-muted-foreground mb-4" />
              <h3 className="text-lg font-medium mb-2">权限限制</h3>
              <p className="text-muted-foreground">只有 HR 用户可以查看此内容</p>
            </div>
          </div>
        )
      case "settings":
        return renderSettingsContent()
      case "faq":
        return renderFAQContent()
      default:
        return renderOverviewContent()
    }
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-3xl font-bold text-foreground">帮助中心</h1>
        <p className="text-muted-foreground mt-2">查找使用指南、常见问题和系统帮助</p>
      </div>

      {/* 主要内容区域 */}
      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        {/* 左侧导航菜单 */}
        <div className="lg:col-span-1">
          <Card className="sticky top-6">
            <CardHeader className="pb-3">
              <CardTitle className="text-lg">帮助主题</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {helpSections
                .filter(section => section.available)
                .map((section) => (
                  <Button
                    key={section.id}
                    variant={activeSection === section.id ? "default" : "ghost"}
                    className="w-full justify-start text-left"
                    onClick={() => setActiveSection(section.id)}
                  >
                    {section.icon}
                    <span className="ml-2">{section.label}</span>
                  </Button>
                ))}
            </CardContent>
          </Card>
        </div>

        {/* 右侧内容区域 */}
        <div className="lg:col-span-3" ref={contentRef}>
          {renderContent()}
        </div>
      </div>
    </div>
  )
} 