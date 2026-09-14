"use client"

import { useState, useEffect } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Users, Building, TrendingUp, CheckCircle, AlertCircle, Award, Target, Eye, Plus } from "lucide-react"
import { statisticsApi, type DashboardStats } from "@/lib/api"
import Link from "next/link"
import { useAppContext } from "@/lib/app-context"
import { LoadingInline } from "@/components/loading"
import { useNotification } from "@/lib/notification-context"
import { formatScore } from "@/lib/utils"

export default function Dashboard() {
  const { getStatusBadge } = useAppContext()
  const { onMessage } = useNotification()
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [loading, setLoading] = useState(true)

  // 获取仪表板统计数据
  const fetchDashboardStats = async () => {
    try {
      const response = await statisticsApi.getDashboard()
      setStats(response.data)
    } catch (error) {
      console.error("获取统计数据失败:", error)
    }
    setLoading(false)
  }

  useEffect(() => {
    fetchDashboardStats()
  }, [])

  // 监听实时通知事件
  useEffect(() => {
    const unsubscribe = onMessage((message) => {
      // 当收到评估相关事件时，重新获取统计数据
      if (message.type.includes('evaluation') || message.type.includes('score')) {
        fetchDashboardStats()
      }
    })

    return unsubscribe
  }, [onMessage])

  if (loading) {
    return <LoadingInline className="py-8" message="加载中..." />
  }

  return (
    <div className="space-y-6 lg:space-y-8">
      {/* 响应式头部 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold text-foreground">仪表板</h1>
          <p className="text-muted-foreground mt-1 sm:mt-2">欢迎使用KPI绩效考核系统</p>
        </div>
      </div>

      {/* 统计卡片 */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-6">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-xs sm:text-sm font-medium">总员工数</CardTitle>
            <Users className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-xl sm:text-2xl font-bold">{stats?.total_employees || 0}</div>
            <p className="text-xs text-muted-foreground">活跃员工</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-xs sm:text-sm font-medium">部门数量</CardTitle>
            <Building className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-xl sm:text-2xl font-bold">{stats?.total_departments || 0}</div>
            <p className="text-xs text-muted-foreground">组织部门</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-xs sm:text-sm font-medium">待处理</CardTitle>
            <AlertCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-xl sm:text-2xl font-bold">{stats?.pending_evaluations || 0}</div>
            <p className="text-xs text-muted-foreground">需要处理</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-xs sm:text-sm font-medium">平均分数</CardTitle>
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-xl sm:text-2xl font-bold">{stats ? formatScore(stats.average_score) : "0"}</div>
            <p className="text-xs text-muted-foreground">整体表现</p>
          </CardContent>
        </Card>
      </div>

      {/* 次要统计 */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 sm:gap-6">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">总评估数</CardTitle>
            <Award className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats?.total_evaluations || 0}</div>
            <p className="text-xs text-muted-foreground">累计考核</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">已完成</CardTitle>
            <CheckCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats?.completed_evaluations || 0}</div>
            <p className="text-xs text-muted-foreground">完成考核</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">完成率</CardTitle>
            <Target className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {stats?.total_evaluations ? Math.round((stats.completed_evaluations / stats.total_evaluations) * 100) : 0}
              %
            </div>
            <p className="text-xs text-muted-foreground">考核完成率</p>
          </CardContent>
        </Card>
      </div>

      {/* 最近活动 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader>
            <CardTitle>
              <div className="flex items-center justify-between">
                <div>最近评估</div>
                {stats?.recent_evaluations && stats.recent_evaluations.length > 0 && (
                  <Link
                    href="/evaluations"
                    className="flex items-center gap-2 text-muted-foreground hover:text-foreground font-normal text-sm leading-none"
                  >
                    <Eye className="w-4 h-4" />
                    查看全部
                  </Link>
                )}
              </div>
            </CardTitle>
          </CardHeader>
          <CardContent className="flex-1 flex flex-col gap-2">
            {stats?.recent_evaluations && stats.recent_evaluations.length > 0 ? (
              <div className="space-y-3">
                {stats.recent_evaluations.slice(0, 5).map(evaluation => (
                  <div
                    key={evaluation.id}
                    className="flex flex-col sm:flex-row sm:items-center sm:justify-between p-3 bg-muted/50 rounded-lg space-y-2 sm:space-y-0"
                  >
                    <div className="flex-1 sm:w-0">
                      <div className="flex flex-col sm:flex-row sm:items-center sm:space-x-2 truncate">
                        <span className="font-medium truncate">{evaluation.employee?.name}</span>
                        <span className="text-sm text-muted-foreground truncate">
                          {evaluation.employee?.department?.name} - {evaluation.employee?.position}
                        </span>
                      </div>
                    </div>
                    <div className="flex items-center justify-between sm:justify-end sm:space-x-2">
                      <div className="text-right">
                        <div className="flex items-center justify-end">
                          <div className="text-xs text-muted-foreground truncate">得分</div>
                          <div className="text-lg font-semibold ml-1 truncate">{formatScore(evaluation.total_score)}</div>
                        </div>
                      </div>
                      <div className="sm:ml-2 truncate">{getStatusBadge(evaluation.status)}</div>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="flex-1 flex items-center justify-center">
                <div className="text-center py-8 text-muted-foreground">暂无评估记录</div>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>快速操作</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <Link href="/evaluations">
              <div className="flex items-center justify-between p-3 bg-blue-50/80 dark:bg-blue-950/50 rounded-lg hover:bg-blue-100/80 dark:hover:bg-blue-950/70 transition-colors cursor-pointer">
                <div className="flex-1">
                  <p className="font-medium">考核管理</p>
                  <p className="text-sm text-muted-foreground">创建和管理员工KPI评估</p>
                </div>
                <div className="flex items-center mx-2 flex-shrink-0 text-xs font-medium text-secondary-foreground/90">
                  <Plus className="w-3 h-3 mr-1" />
                  新建
                </div>
              </div>
            </Link>
            <Link href="/templates">
              <div className="flex items-center justify-between p-3 bg-green-50/80 dark:bg-green-950/50 rounded-lg hover:bg-green-100/80 dark:hover:bg-green-950/70 transition-colors cursor-pointer">
                <div className="flex-1">
                  <p className="font-medium">KPI模板</p>
                  <p className="text-sm text-muted-foreground">创建和管理KPI考核模板</p>
                </div>
                <div className="flex items-center mx-2 flex-shrink-0 text-xs font-medium text-secondary-foreground/90">
                  可用
                </div>
              </div>
            </Link>
            <Link href="/statistics">
              <div className="flex items-center justify-between p-3 bg-purple-50/80 dark:bg-purple-950/50 rounded-lg hover:bg-purple-100/80 dark:hover:bg-purple-950/70 transition-colors cursor-pointer">
                <div className="flex-1">
                  <p className="font-medium">统计分析</p>
                  <p className="text-sm text-muted-foreground">查看绩效统计和分析报告</p>
                </div>
                <div className="flex items-center mx-2 flex-shrink-0 text-xs font-medium text-secondary-foreground/90">
                  可用
                </div>
              </div>
            </Link>
            <Link href="/employees">
              <div className="flex items-center justify-between p-3 bg-orange-50/80 dark:bg-orange-950/50 rounded-lg hover:bg-orange-100/80 dark:hover:bg-orange-950/70 transition-colors cursor-pointer">
                <div className="flex-1">
                  <p className="font-medium">员工管理</p>
                  <p className="text-sm text-muted-foreground">管理员工信息和组织架构</p>
                </div>
                <div className="flex items-center mx-2 flex-shrink-0 text-xs font-medium text-secondary-foreground/90">
                  可用
                </div>
              </div>
            </Link>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
