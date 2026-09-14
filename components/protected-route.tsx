"use client"

import { useEffect } from "react"
import { useRouter, usePathname } from "next/navigation"
import { useAuth } from "@/lib/auth-context"
import Loading from "./loading"
import { useDootaskContext } from "@/lib/dootask-context"
import { AlertCircle } from "lucide-react"
import { Button } from "./ui/button"

interface ProtectedRouteProps {
  children: React.ReactNode
  requireAuth?: boolean
  redirectTo?: string
}

export default function ProtectedRoute({
  children,
  requireAuth = true,
  redirectTo = "/auth/login",
}: ProtectedRouteProps) {
  const { user, loading, isHR, logout } = useAuth()
  const { loading: dooTaskLoading, error: dooTaskError } = useDootaskContext()
  const router = useRouter()
  const pathname = usePathname()

  // 监听认证状态
  useEffect(() => {
    if (typeof window === "undefined") return

    window.addEventListener("auth_unauthorized", logout)
    return () => {
      window.removeEventListener("auth_unauthorized", logout)
    }
  }, [logout])

  // 监听用户状态
  useEffect(() => {
    if (loading) return

    // 如果需要认证但用户未登录，重定向到登录页
    if (requireAuth && !user) {
      router.push(redirectTo)
      return
    }

    // 如果不需要认证且用户已登录，且当前在认证页面，重定向到首页
    if (!requireAuth && user && pathname.startsWith("/auth")) {
      router.push(isHR ? "/" : "/evaluations")
      return
    }
  }, [user, loading, requireAuth, router, redirectTo, pathname, isHR])

  // 显示加载状态
  if (loading || dooTaskLoading) {
    return <Loading delay={1000} />
  }

  // 显示错误状态
  if (dooTaskError) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="text-center">
          <AlertCircle className="w-16 h-16 mx-auto mb-4 text-red-400 animate-bounce" />
          <h2 className="text-2xl font-semibold text-red-500 mb-2">系统异常</h2>
          <p className="text-gray-600 mb-4">{dooTaskError}</p>
          <Button onClick={() => window.location.reload()}>刷新</Button>
        </div>
      </div>
    )
  }

  // 如果需要认证但用户未登录，不渲染内容
  if (requireAuth && !user) {
    return null
  }

  // 如果不需要认证但用户已登录，且在认证页面，不渲染内容
  if (!requireAuth && user && pathname.startsWith("/auth")) {
    return null
  }

  return <>{children}</>
}
