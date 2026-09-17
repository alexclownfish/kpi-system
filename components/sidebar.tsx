"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { cn } from "@/lib/utils"
import {
  Users,
  Building,
  ClipboardList,
  FileText,
  BarChart3,
  Settings,
  Home,
  Menu,
  X,
  HelpCircle,
  MessageSquare,
  Database,
  Scale,
	ShieldCheck,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"
import { useEffect, useMemo } from "react"
import { useAuth } from "@/lib/auth-context"
import { Button } from "./ui/button"
import { useRouter } from "next/navigation"
import { Badge } from "./ui/badge"
import { useDootaskContext } from "@/lib/dootask-context"
import { useUnreadContext } from "@/lib/unread-context"
import { getDefaultLandingPath, getRoleLabel, normalizeRoleCode } from "@/lib/access-control"

interface SidebarProps {
  isMobileMenuOpen: boolean
  setIsMobileMenuOpen: (open: boolean) => void
}

interface NavigationItem {
  name: string
  href: string
  icon: LucideIcon
  permission?: string
  badge?: number
  hidden?: boolean
}

interface NavigationSection {
  category: string
  items: NavigationItem[]
}

export function Sidebar({ isMobileMenuOpen, setIsMobileMenuOpen }: SidebarProps) {
  const pathname = usePathname()
  const router = useRouter()
  const { user: currentUser } = useAuth()
  const { isDootask } = useDootaskContext()
  const { unreadInvitations, unreadEvaluations } = useUnreadContext()

  const getRoleBadge = (role: string) => {
    const code = normalizeRoleCode(role)
    const variant = code === "super_admin" ? "destructive" : code === "employee" ? "outline" : "secondary"
    return <Badge variant={variant}>{getRoleLabel(code)}</Badge>
  }

  const navigation = useMemo(() => {
    const permissions = currentUser?.permissions || []
    const menus: NavigationSection[] = [
      {
        category: "工作台",
        items: [
          { name: "仪表板", href: "/", icon: Home, permission: "report:company" },
          { name: "考核管理", href: "/evaluations", icon: FileText, permission: "assessment:view", badge: unreadEvaluations || undefined },
          { name: "邀请评分", href: "/invitations", icon: MessageSquare, permission: "review:view", badge: unreadInvitations || undefined },
          { name: "统计分析", href: "/statistics", icon: BarChart3, permission: "report:company" },
        ],
      },
      {
        category: "组织与绩效",
        items: [
          { name: "部门管理", href: "/departments", icon: Building, permission: "department:view" },
          { name: "员工管理", href: "/employees", icon: Users, permission: "employee:create" },
		  { name: "用户角色", href: "/roles", icon: ShieldCheck, permission: "role:view" },
          { name: "KPI 模板", href: "/templates", icon: ClipboardList, permission: "kpi:view" },
          { name: "绩效规则", href: "/performance-rules", icon: Scale, permission: "kpi:edit" },
        ],
      },
      {
        category: "系统",
        items: [
          { name: "备份还原", href: "/backup", icon: Database, permission: "system:edit" },
          { name: "系统设置", href: "/settings", icon: Settings, hidden: isDootask },
          { name: "帮助中心", href: "/help", icon: HelpCircle },
        ],
      },
    ]

    return menus
      .map(menu => ({
        ...menu,
        items: menu.items.filter(item => !item.hidden && (!item.permission || permissions.includes(item.permission))),
      }))
      .filter(menu => menu.items.length > 0)
  }, [currentUser?.permissions, isDootask, unreadInvitations, unreadEvaluations])

  // 点击导航项时关闭移动端菜单
  const handleNavClick = () => {
    setIsMobileMenuOpen(false)
  }

  useEffect(() => {
    if (!currentUser) return
    const restrictedRoots = ["/", "/evaluations", "/invitations", "/statistics", "/departments", "/employees", "/roles", "/templates", "/performance-rules", "/backup"]
    const currentRoot = restrictedRoots.find(root => root === "/" ? pathname === "/" : pathname === root || pathname.startsWith(`${root}/`))
    if (!currentRoot) return
    const allowed = navigation.flatMap(section => section.items).some(item => item.href === currentRoot)
    if (!allowed) router.replace(getDefaultLandingPath(currentUser.permissions))
  }, [currentUser, navigation, pathname, router])

  // 监听屏幕尺寸变化，在桌面端自动关闭移动菜单
  useEffect(() => {
    const handleResize = () => {
      if (window.innerWidth >= 1024) {
        setIsMobileMenuOpen(false)
      }
    }

    window.addEventListener("resize", handleResize)
    return () => window.removeEventListener("resize", handleResize)
  }, [setIsMobileMenuOpen])

  return (
    <>
      {/* 移动端遮罩层 */}
      {isMobileMenuOpen && (
        <div className="fixed inset-0 bg-black/50 z-10 lg:hidden" onClick={() => setIsMobileMenuOpen(false)} />
      )}

      {/* 侧边栏 */}
      <div
        className={cn(
          "fixed lg:static inset-y-0 left-0 z-20 w-64 h-screen lg:h-full bg-sidebar shadow-lg transform transition-transform duration-300 ease-in-out lg:transform-none flex flex-col pt-[var(--safe-area-top,0px)] pb-[var(--safe-area-bottom,0px)]",
          isMobileMenuOpen ? "translate-x-0" : "-translate-x-full lg:translate-x-0"
        )}
      >
        {/* 头部 - 移动端显示关闭按钮 */}
        <div className="flex items-center justify-between pl-6 pr-2 lg:pr-6 py-4 lg:justify-start flex-shrink-0">
          <div className="flex flex-col gap-0.5">
            <h1 className="text-xl font-bold text-sidebar-foreground">KPI考核系统</h1>
            {currentUser && (
              <div className="text-sm text-sidebar-foreground/70 flex items-center gap-2">
                <div>{[currentUser.name, currentUser.department?.name].filter(Boolean).join(" - ")}</div>
                <div>{getRoleBadge(currentUser.role)}</div>
              </div>
            )}
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setIsMobileMenuOpen(false)}
            className="lg:hidden p-1 rounded-md hover:bg-sidebar-accent"
          >
            <X className="text-sidebar-foreground/70" />
          </Button>
        </div>

        {/* 导航菜单 - 可滚动区域 */}
        <nav className="flex-1 overflow-y-auto py-1">
          {navigation.map(section => (
            <div key={section.category} className="mb-2">
              {/* 分类标题 */}
              <div className="px-6 py-2 text-xs font-semibold text-sidebar-foreground/50 uppercase tracking-wider">
                {section.category}
              </div>

              {/* 分类下的菜单项 */}
              {section.items.map(item => {
                const isActive = pathname === item.href
                return (
                  <Link
                    key={item.name}
                    href={item.href}
                    onClick={handleNavClick}
                    className={cn(
                      "flex items-center justify-between px-6 py-3 text-sm font-medium transition-colors border-r-2 border-transparent",
                      isActive
                        ? "bg-sidebar-accent border-sidebar-primary"
                        : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-foreground"
                    )}
                  >
                    <div className="flex items-center">
                      <item.icon className="w-5 h-5 mr-3" />
                      {item.name}
                    </div>
                    {"badge" in item && item.badge && (
                      <Badge
                        variant="destructive"
                        className="ml-2 min-w-[20px] h-5 flex items-center justify-center text-xs"
                      >
                        {item.badge}
                      </Badge>
                    )}
                  </Link>
                )
              })}
            </div>
          ))}
        </nav>
      </div>
    </>
  )
}

// 移动端头部组件
export function MobileHeader({ onMenuClick }: { onMenuClick: () => void }) {
  const { unreadEvaluations, unreadInvitations } = useUnreadContext()
  return (
    <div className="lg:hidden bg-background shadow-sm border-b border-border flex-shrink-0 pt-[var(--safe-area-top,0px)]">
      <div className="flex items-center justify-between px-4 py-1.5">
        <button onClick={onMenuClick} className="p-2 rounded-md hover:bg-accent relative">
          <Menu className="w-6 h-6 text-muted-foreground" />
          {unreadEvaluations + unreadInvitations > 0 && (
            <div className="absolute top-0 left-5.5 min-w-6 h-5 px-1.5 flex items-center justify-center text-xs bg-destructive/90 text-white rounded-md scale-95">
              {unreadEvaluations + unreadInvitations}
            </div>
          )}
        </button>
        <h1 className="text-lg font-semibold text-foreground">KPI考核系统</h1>
        <div className="w-10 h-10"></div>
      </div>
    </div>
  )
}
