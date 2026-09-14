"use client"

import { useState, useEffect } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { Circle, CircleCheck } from "lucide-react"
import { RefreshCw, CheckCircle, LogOut, Monitor, Sun, Moon, Palette, Shield } from "lucide-react"
import { useAuth } from "@/lib/auth-context"
import { useAppContext } from "@/lib/app-context"
import { useTheme } from "@/lib/theme-context"
import { settingsApi } from "@/lib/api"
import { toast } from "sonner"
import { cn } from "@/lib/utils"

type SettingTab = "appearance" | "system" | "logout"

export default function SettingsPage() {
  const { logout, isHR } = useAuth()
  const { Confirm } = useAppContext()
  const { theme, setTheme } = useTheme()
  const [allowRegistration, setAllowRegistration] = useState(true)
  const [loading, setLoading] = useState(false)
  const [activeTab, setActiveTab] = useState<SettingTab>("appearance")

  // 初始化设置状态
  useEffect(() => {
    const fetchSettings = async () => {
      if (!isHR) return // 非HR用户不需要加载系统设置
      
      try {
        setLoading(true)
        const response = await settingsApi.get()
        setAllowRegistration(response.data.allow_registration)
      } catch (error) {
        console.error("获取设置失败:", error)
        toast.error("获取设置失败")
      } finally {
        setLoading(false)
      }
    }
    fetchSettings()
  }, [isHR])

  // 保存设置
  const handleSaveSettings = async () => {
    if (!isHR) {
      toast.error("只有HR用户可以保存系统设置")
      return
    }

    setLoading(true)
    try {
      const response = await settingsApi.update({
        allow_registration: allowRegistration,
      })
      toast.success(response.message || "设置保存成功！")
    } catch (error) {
      console.error("保存设置失败:", error)
      toast.error("保存设置失败，请重试。")
    } finally {
      setLoading(false)
    }
  }

  // 退出登录
  const handleLogout = async () => {
    const result = await Confirm("退出登录", "确定要退出当前账户吗？")
    if (result) {
      logout()
    }
  }

  // 菜单项配置
  const menuItems = [
    {
      id: "appearance" as SettingTab,
      label: "外观设置",
      icon: <Palette className="w-4 h-4" />,
      available: true,
    },
    {
      id: "system" as SettingTab,
      label: "系统设置",
      icon: <Shield className="w-4 h-4" />,
      available: isHR,
    },
    {
      id: "logout" as SettingTab,
      label: "退出登录",
      icon: <LogOut className="w-4 h-4" />,
      available: true,
      action: handleLogout,
    },
  ]

  // 渲染外观设置内容
  const renderAppearanceContent = () => (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center">
          <Palette className="w-5 h-5 mr-2" />
          外观设置
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <Label htmlFor="theme" className="text-sm font-medium mb-2 block">
            主题模式
          </Label>
          <div className="flex gap-4">
            {/* 浅色卡片 */}
            <button
              type="button"
              className={cn(
                "group flex-1 rounded-xl flex flex-col items-center p-0 overflow-hidden transition-all cursor-pointer shadow-sm hover:shadow-md",
                theme === "light"
                  ? "ring-1 ring-primary bg-background"
                  : "bg-background hover:bg-muted/50"
              )}
              onClick={() => setTheme("light")}
              aria-label="浅色模式"
            >
              {/* 预览区 */}
              <div className="w-full h-16 flex items-center justify-center bg-white border-b border-muted">
                <div className="w-8 h-8 rounded bg-background flex items-center justify-center">
                  <Sun className="w-5 h-5 text-yellow-400" />
                </div>
              </div>
              {/* 名称区 */}
              <div className="flex flex-col items-center py-3">
                <div className="flex items-center gap-1.5">
                  {theme === "light"
                    ? <CircleCheck className="w-4 h-4 text-primary" />
                    : <Circle className="w-4 h-4 text-muted-foreground" />}
                  <span className="font-medium text-sm text-foreground">浅色</span>
                </div>
              </div>
            </button>
            {/* 深色卡片 */}
            <button
              type="button"
              className={cn(
                "group flex-1 rounded-xl flex flex-col items-center p-0 overflow-hidden transition-all cursor-pointer shadow-sm hover:shadow-md",
                theme === "dark"
                  ? "ring-1 ring-primary bg-background"
                  : "bg-background hover:bg-muted/50"
              )}
              onClick={() => setTheme("dark")}
              aria-label="深色模式"
            >
              {/* 预览区 */}
              <div className="w-full h-16 flex items-center justify-center bg-zinc-900 border-b border-muted">
                <div className="w-8 h-8 rounded bg-zinc-800 flex items-center justify-center">
                  <Moon className="w-5 h-5 text-blue-400" />
                </div>
              </div>
              {/* 名称区 */}
              <div className="flex flex-col items-center py-3">
                <div className="flex items-center gap-1.5">
                  {theme === "dark"
                    ? <CircleCheck className="w-4 h-4 text-primary" />
                    : <Circle className="w-4 h-4 text-muted-foreground" />}
                  <span className="font-medium text-sm text-foreground">深色</span>
                </div>
              </div>
            </button>
            {/* 跟随系统卡片 */}
            <button
              type="button"
              className={cn(
                "group flex-1 rounded-xl flex flex-col items-center p-0 overflow-hidden transition-all cursor-pointer shadow-sm hover:shadow-md",
                theme === "system"
                  ? "ring-1 ring-primary bg-background"
                  : "bg-background hover:bg-muted/50"
              )}
              onClick={() => setTheme("system")}
              aria-label="跟随系统"
            >
              {/* 预览区 */}
              <div className="w-full h-16 flex items-center justify-center bg-gradient-to-r from-white via-zinc-900 to-white border-b border-muted">
                <div className="w-8 h-8 rounded bg-gradient-to-br from-white via-zinc-900 to-zinc-800 flex items-center justify-center">
                  <Monitor className="w-5 h-5 text-green-500" />
                </div>
              </div>
              {/* 名称区 */}
              <div className="flex flex-col items-center py-3 max-w-full overflow-x-auto">
                <div className="flex items-center gap-1.5 px-2">
                  {theme === "system"
                    ? <CircleCheck className="w-4 h-4 text-primary shrink-0" />
                    : <Circle className="w-4 h-4 text-muted-foreground shrink-0" />}
                  <span className="font-medium text-sm text-foreground">跟随系统</span>
                </div>
              </div>
            </button>
          </div>
        </div>
        <div className="p-4 bg-muted/50 rounded-lg">
          <h3 className="text-sm font-medium mb-2 text-foreground">主题说明</h3>
          <ul className="text-sm space-y-1 text-muted-foreground">
            <li>• 浅色模式：使用亮色背景和深色文字</li>
            <li>• 深色模式：使用深色背景和浅色文字，有助于减少眼部疲劳</li>
            <li>• 跟随系统：自动根据您的设备系统设置切换主题</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )

  // 渲染系统设置内容
  const renderSystemContent = () => (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center">
          <Shield className="w-5 h-5 mr-2" />
          系统设置
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="flex items-center justify-between p-4 bg-muted/50 rounded-lg">
          <div className="flex-1">
            <Label htmlFor="allow_registration" className="text-sm font-medium">
              允许用户注册
            </Label>
            <p className="text-sm text-muted-foreground mt-1">
              开启后，新用户可以自行注册账户；关闭后，只能由管理员创建账户。
            </p>
          </div>
          <Switch id="allow_registration" checked={allowRegistration} onCheckedChange={setAllowRegistration} />
        </div>
        <div className="p-4 bg-muted/50 rounded-lg">
          <h3 className="text-sm font-medium mb-2 text-foreground">功能说明</h3>
          <ul className="text-sm space-y-1 text-muted-foreground">
            <li>• 开启注册：用户可以通过注册页面创建新账户</li>
            <li>• 关闭注册：注册页面将显示&quot;暂不开放注册&quot;的提示</li>
            <li>• 只有HR用户可以修改此设置</li>
          </ul>
        </div>
        <div className="flex justify-end">
          <Button onClick={handleSaveSettings} disabled={loading}>
            {loading ? <RefreshCw className="w-4 h-4 mr-2 animate-spin" /> : <CheckCircle className="w-4 h-4 mr-2" />}
            保存设置
          </Button>
        </div>
      </CardContent>
    </Card>
  )

  // 处理菜单项点击
  const handleMenuClick = (item: typeof menuItems[0]) => {
    if (item.action) {
      item.action()
    } else {
      setActiveTab(item.id)
    }
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-3xl font-bold text-foreground">系统设置</h1>
        <p className="text-muted-foreground mt-2">管理您的个人偏好和系统配置</p>
      </div>

      {/* 主要内容区域 */}
      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        {/* 左侧导航菜单 - 小屏幕时显示在上方 */}
        <div className="lg:col-span-1">
          <Card className="sticky top-6">
            <CardHeader className="pb-3">
              <CardTitle className="text-lg">设置菜单</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {menuItems
                .filter(item => item.available)
                .map((item) => (
                  <Button
                    key={item.id}
                    variant={activeTab === item.id ? "default" : "ghost"}
                    className={cn(
                      "w-full justify-start text-left",
                      item.id === "logout" && "text-destructive hover:text-destructive/90 hover:bg-destructive/10"
                    )}
                    onClick={() => handleMenuClick(item)}
                  >
                    {item.icon}
                    <span className="ml-2">{item.label}</span>
                  </Button>
                ))}
            </CardContent>
          </Card>
        </div>

        {/* 右侧内容区域 */}
        <div className="lg:col-span-3">
          {activeTab === "appearance" && renderAppearanceContent()}
          {activeTab === "system" && isHR && renderSystemContent()}
          {activeTab === "system" && !isHR && (
            <div className="flex items-center justify-center h-64">
              <div className="text-center">
                <Shield className="w-12 h-12 mx-auto text-muted-foreground mb-4" />
                <h3 className="text-lg font-medium mb-2">权限不足</h3>
                <p className="text-muted-foreground">只有HR用户可以访问系统设置</p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
