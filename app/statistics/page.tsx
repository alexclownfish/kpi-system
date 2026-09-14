"use client"

import { useState, useEffect, useCallback } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  PieChart,
  Pie,
  Cell,
  LineChart,
  Line,
  Area,
  AreaChart,
} from "recharts"
import {
  ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart"
import {
  TrendingUp,
  Users,
  Award,
  Target,
  BarChart3,
  PieChart as PieChartIcon,
  LineChart as LineChartIcon,
  Calendar,
  Download,
} from "lucide-react"
import { statisticsApi, exportApi, type DashboardStats, type StatisticsResponse } from "@/lib/api"
import { getPeriodValue, formatScore } from "@/lib/utils"
import { useAppContext } from "@/lib/app-context"
import { LoadingInline } from "@/components/loading"
import { useNotification } from "@/lib/notification-context"
import { downloadUrl } from "@dootask/tools"

export default function StatisticsPage() {
  const { getStatusBadge } = useAppContext()
  const { onMessage } = useNotification()
  const [dashboardStats, setDashboardStats] = useState<DashboardStats | null>(null)
  const [statisticsData, setStatisticsData] = useState<StatisticsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  // 获取默认时间信息
  const getDefaultPeriodInfo = useCallback((period: string) => {
    const now = new Date()
    
    switch (period) {
      case "yearly":
        // 年度：默认选择今年
        return {
          year: now.getFullYear(),
          month: now.getMonth() + 1,
          quarter: Math.floor(now.getMonth() / 3) + 1
        }
      case "quarterly":
        // 季度：默认选择上个季度
        const lastQuarter = new Date(now.getFullYear(), now.getMonth() - 3, 1)
        return {
          year: lastQuarter.getFullYear(),
          month: lastQuarter.getMonth() + 1,
          quarter: Math.floor(lastQuarter.getMonth() / 3) + 1
        }
      case "monthly":
      default:
        // 月度：默认选择上个月
        const lastMonth = new Date(now.getFullYear(), now.getMonth() - 1, 1)
        return {
          year: lastMonth.getFullYear(),
          month: lastMonth.getMonth() + 1,
          quarter: Math.floor(lastMonth.getMonth() / 3) + 1
        }
    }
  }, [])

  const defaultInfo = getDefaultPeriodInfo("monthly")
  
  const [selectedYear, setSelectedYear] = useState(defaultInfo.year)
  const [selectedPeriod, setSelectedPeriod] = useState("monthly")
  const [selectedMonth, setSelectedMonth] = useState(defaultInfo.month)
  const [selectedQuarter, setSelectedQuarter] = useState(defaultInfo.quarter)

  // 图表配置 - 使用CSS变量以支持主题切换
  const chartConfig = {
    total: {
      label: "总数",
    },
    completed: {
      label: "已完成", 
    },
    pending: {
      label: "待处理",
    },
    avg_score: {
      label: "平均分",
    },
    evaluations: {
      label: "考核数量",
    },
    completion_rate: {
      label: "完成率 %",
    },
  } satisfies ChartConfig

  // 获取统计数据
  const fetchStatisticsData = useCallback(async () => {
    try {
      setLoading(true)
      const filterParams = {
        year: selectedYear.toString(),
        period: selectedPeriod,
        month: selectedPeriod === "monthly" ? selectedMonth.toString() : undefined,
        quarter: selectedPeriod === "quarterly" ? selectedQuarter.toString() : undefined,
      }

      const [dashboardResponse, statisticsResponse] = await Promise.all([
        statisticsApi.getDashboard(filterParams),
        statisticsApi.getData(filterParams),
      ])

      setDashboardStats(dashboardResponse.data)
      setStatisticsData(statisticsResponse.data)
    } catch (error) {
      console.error("获取统计数据失败:", error)
    } finally {
      setLoading(false)
    }
  }, [selectedYear, selectedPeriod, selectedMonth, selectedQuarter])

  // 当周期类型改变时，自动调整默认时间
  useEffect(() => {
    const newDefaultInfo = getDefaultPeriodInfo(selectedPeriod)
    setSelectedYear(newDefaultInfo.year)
    setSelectedMonth(newDefaultInfo.month)
    setSelectedQuarter(newDefaultInfo.quarter)
  }, [selectedPeriod, getDefaultPeriodInfo])

  useEffect(() => {
    fetchStatisticsData()
  }, [selectedYear, selectedPeriod, selectedMonth, selectedQuarter, fetchStatisticsData])

  // 导出报告
  const handleExport = async () => {
    try {
      const response = await exportApi.period(selectedPeriod, {
        year: selectedYear.toString(),
        month: selectedPeriod === "monthly" ? selectedMonth.toString() : undefined,
        quarter: selectedPeriod === "quarterly" ? selectedQuarter.toString() : undefined,
      })

      // 直接跳转到下载URL
      try {
        await downloadUrl(response.file_url)
      } catch {
        window.open(response.file_url, "_blank")
      }

      // 显示成功消息
      console.log(response.message)
    } catch (error) {
      console.error("导出失败:", error)
    }
  }

  // 监听实时通知事件
  useEffect(() => {
    const unsubscribe = onMessage((message) => {
      // 当收到评估相关事件时，重新获取统计数据
      if (message.type.includes('evaluation') || message.type.includes('score')) {
        fetchStatisticsData()
      }
    })

    return unsubscribe
  }, [onMessage, fetchStatisticsData])

  if (loading) {
    return <LoadingInline className="py-8" message="加载中..." />
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center flex-wrap gap-4">
        <div>
          <h1 className="text-3xl font-bold text-foreground">统计分析</h1>
          <p className="text-muted-foreground mt-2">
            绩效考核数据分析与报告 - {selectedYear}年
            {selectedPeriod === "monthly"
              ? `${selectedMonth}月`
              : selectedPeriod === "quarterly"
              ? `第${selectedQuarter}季度`
              : "全年"}
            数据
          </p>
        </div>
        <div className="flex items-center flex-wrap gap-2 lg:mt-8">
          <Select
            value={selectedPeriod}
            onValueChange={value => {
              setSelectedPeriod(value)
              // 重置月份和季度选择
              if (value === "monthly") setSelectedMonth(new Date().getMonth() + 1)
              if (value === "quarterly") setSelectedQuarter(Math.floor(new Date().getMonth() / 3) + 1)
            }}
          >
            <SelectTrigger className="w-auto">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="yearly">年度</SelectItem>
              <SelectItem value="quarterly">季度</SelectItem>
              <SelectItem value="monthly">月度</SelectItem>
            </SelectContent>
          </Select>
          <Select value={selectedYear.toString()} onValueChange={value => setSelectedYear(parseInt(value))}>
            <SelectTrigger className="w-auto">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Array.from({ length: 10 }, (_, i) => {
                const year = new Date().getFullYear() - i
                return (
                  <SelectItem key={year} value={year.toString()}>
                    {year}年
                  </SelectItem>
                )
              })}
            </SelectContent>
          </Select>
          {selectedPeriod === "monthly" && (
            <Select value={selectedMonth.toString()} onValueChange={value => setSelectedMonth(parseInt(value))}>
              <SelectTrigger className="w-auto">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Array.from({ length: 12 }, (_, i) => (
                  <SelectItem key={i + 1} value={(i + 1).toString()}>
                    {i + 1}月
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          {selectedPeriod === "quarterly" && (
            <Select value={selectedQuarter.toString()} onValueChange={value => setSelectedQuarter(parseInt(value))}>
              <SelectTrigger className="w-auto">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1">第1季度</SelectItem>
                <SelectItem value="2">第2季度</SelectItem>
                <SelectItem value="3">第3季度</SelectItem>
                <SelectItem value="4">第4季度</SelectItem>
              </SelectContent>
            </Select>
          )}
          <Button onClick={handleExport}>
            <Download className="w-4 h-4 mr-2" />
            导出报告
          </Button>
        </div>
      </div>

      {/* 总体统计卡片 */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">总员工数</CardTitle>
            <Users className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{dashboardStats?.total_employees || 0}</div>
            <p className="text-xs text-muted-foreground">活跃员工数量</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">总考核数</CardTitle>
            <Award className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{dashboardStats?.total_evaluations || 0}</div>
            <p className="text-xs text-muted-foreground">累计考核项目</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">完成率</CardTitle>
            <Target className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {dashboardStats?.total_evaluations
                ? Math.round((dashboardStats.completed_evaluations / dashboardStats.total_evaluations) * 100)
                : 0}
              %
            </div>
            <p className="text-xs text-muted-foreground">考核完成比例</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">平均分</CardTitle>
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{dashboardStats ? formatScore(dashboardStats.average_score) : 0}</div>
            <p className="text-xs text-muted-foreground">总体平均得分</p>
          </CardContent>
        </Card>
      </div>

      {/* 数据分析图表 */}
      <Tabs defaultValue="departments" className="space-y-4">
        <TabsList className="gap-1 max-w-full overflow-x-auto justify-start">
          <TabsTrigger value="departments">部门分析</TabsTrigger>
          <TabsTrigger value="trends">趋势分析</TabsTrigger>
          <TabsTrigger value="distribution">分数分布</TabsTrigger>
          <TabsTrigger value="performance">绩效排名</TabsTrigger>
        </TabsList>

        <TabsContent value="departments" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center">
                <BarChart3 className="w-5 h-5 mr-2" />
                部门考核统计
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ChartContainer config={chartConfig} className="h-80 w-full">
                <BarChart data={statisticsData?.departmentStats || []}>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="name" tickLine={false} axisLine={false} />
                  <YAxis tickLine={false} axisLine={false} />
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <Bar dataKey="total" fill="var(--chart-1)" />
                  <Bar dataKey="completed" fill="var(--chart-2)" />
                  <Bar dataKey="pending" fill="var(--chart-3)" />
                </BarChart>
              </ChartContainer>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>部门平均分对比</CardTitle>
            </CardHeader>
            <CardContent>
              <ChartContainer config={chartConfig} className="h-64 w-full">
                <BarChart data={statisticsData?.departmentStats || []}>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="name" tickLine={false} axisLine={false} />
                  <YAxis domain={[0, 100]} tickLine={false} axisLine={false} />
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <Bar dataKey="avg_score" fill="var(--chart-4)" />
                </BarChart>
              </ChartContainer>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="trends" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center">
                <LineChartIcon className="w-5 h-5 mr-2" />
                月度趋势分析
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ChartContainer config={chartConfig} className="h-80 w-full">
                <LineChart data={statisticsData?.monthlyTrends || []}>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="month" tickLine={false} axisLine={false} />
                  <YAxis yAxisId="left" tickLine={false} axisLine={false} />
                  <YAxis yAxisId="right" orientation="right" tickLine={false} axisLine={false} />
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <Line yAxisId="left" type="monotone" dataKey="evaluations" stroke="var(--chart-1)" strokeWidth={2} />
                  <Line yAxisId="right" type="monotone" dataKey="avg_score" stroke="var(--chart-4)" strokeWidth={2} />
                </LineChart>
              </ChartContainer>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>完成率趋势</CardTitle>
            </CardHeader>
            <CardContent>
              <ChartContainer config={chartConfig} className="h-64 w-full">
                <AreaChart data={statisticsData?.monthlyTrends || []}>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="month" tickLine={false} axisLine={false} />
                  <YAxis tickLine={false} axisLine={false} />
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <Area type="monotone" dataKey="completion_rate" stroke="var(--chart-5)" fill="var(--chart-5)" fillOpacity={0.6} />
                </AreaChart>
              </ChartContainer>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="distribution" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center">
                <PieChartIcon className="w-5 h-5 mr-2" />
                分数分布
              </CardTitle>
            </CardHeader>
            <CardContent>
              {/* 检查是否有有效数据 */}
              {(statisticsData?.scoreDistribution || []).every(item => item.count === 0) ? (
                <div className="h-80 flex items-center justify-center">
                  <div className="text-center">
                    <PieChartIcon className="w-12 h-12 mx-auto text-muted-foreground mb-4" />
                    <p className="text-muted-foreground">暂无分数分布数据</p>
                    <p className="text-sm text-muted-foreground mt-2">当前时期内没有完成的考核记录</p>
                  </div>
                </div>
              ) : (
                <ChartContainer config={chartConfig} className="h-80 w-full">
                  <PieChart>
                    <Pie
                      data={(statisticsData?.scoreDistribution || []).map(item => ({
                        ...item,
                        name: item.range + "分",
                      }))}
                      cx="50%"
                      cy="50%"
                      labelLine={false}
                      label={({ range, count }) => count > 0 ? `${range}分: ${count}人` : null}
                      outerRadius={80}
                      fill="var(--chart-1)"
                      dataKey="count"
                    >
                      {(statisticsData?.scoreDistribution || []).map((entry, index) => (
                        <Cell key={`cell-${index}`} fill={entry.color} />
                      ))}
                    </Pie>
                    <ChartTooltip content={<ChartTooltipContent/>} />
                  </PieChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="performance" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center">
                <Award className="w-5 h-5 mr-2" />
                绩效排名
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>排名</TableHead>
                      <TableHead>姓名</TableHead>
                      <TableHead>部门</TableHead>
                      <TableHead>平均分</TableHead>
                      <TableHead>考核次数</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(statisticsData?.topPerformers?.length || 0) === 0 && (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center text-muted-foreground py-4">暂无数据</TableCell>
                      </TableRow>
                    )}
                    {(statisticsData?.topPerformers || []).map((performer, index) => (
                      <TableRow key={performer.name}>
                        <TableCell>
                          <div className="flex items-center">
                            {index < 3 && <Award className="w-4 h-4 mr-1 text-yellow-500" />}
                            {index + 1}
                          </div>
                        </TableCell>
                        <TableCell className="font-medium">{performer.name}</TableCell>
                        <TableCell>{performer.department}</TableCell>
                        <TableCell>
                          <div className="text-lg font-semibold">{formatScore(performer.score)}</div>
                        </TableCell>
                        <TableCell>{performer.evaluations}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* 最近考核记录 */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center">
            <Calendar className="w-5 h-5 mr-2" />
            最近考核记录
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>员工</TableHead>
                  <TableHead>部门</TableHead>
                  <TableHead>考核模板</TableHead>
                  <TableHead>周期</TableHead>
                  <TableHead>得分</TableHead>
                  <TableHead>状态</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(statisticsData?.recentEvaluations?.length || 0) === 0 && (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-muted-foreground py-4">暂无数据</TableCell>
                  </TableRow>
                )}
                {(statisticsData?.recentEvaluations || []).map(evaluation => (
                  <TableRow key={evaluation.id}>
                    <TableCell className="font-medium">{evaluation.employee}</TableCell>
                    <TableCell>{evaluation.department}</TableCell>
                    <TableCell>{evaluation.template}</TableCell>
                    <TableCell>
                      {getPeriodValue(evaluation)}
                    </TableCell>
                    <TableCell>
                       <div className="text-lg font-semibold">{formatScore(evaluation.score)}</div>
                    </TableCell>
                    <TableCell>{getStatusBadge(evaluation.status)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
