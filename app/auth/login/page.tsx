"use client"

import { useEffect, useState } from "react"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "@/lib/auth-context"
import { useAppContext } from "@/lib/app-context"
import { toast } from "sonner"
import Loading from "@/components/loading"
import { useDootaskContext } from "@/lib/dootask-context"
import { useRouter } from "next/navigation"

export default function LoginPage() {
  const router = useRouter()
  const { login } = useAuth()
  const { Alert } = useAppContext()
  const { loading: dooTaskLoading, dooTaskUser } = useDootaskContext()
  const [loading, setLoading] = useState(false)
  const [formData, setFormData] = useState({
    email: "",
    password: "",
  })

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)

    try {
      await login({
        email: formData.email,
        password: formData.password,
      })
      toast.success("登录成功")
    } catch (error: unknown) {
      let errorMessage = "登录失败，请重试"

      if (error && typeof error === "object" && "response" in error) {
        const response = (error as { response?: { data?: { error?: string } } }).response
        if (response?.data?.error) {
          errorMessage = response.data.error
        }
      }

      await Alert("登录失败", errorMessage)
    } finally {
      setLoading(false)
    }
  }

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target
    setFormData(prev => ({ ...prev, [name]: value }))
  }

  useEffect(() => {
    if (dooTaskUser) {
      router.push("/evaluations")
    }
  }, [dooTaskUser, router])

  if (dooTaskLoading) {
    return <Loading />
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background px-4">
      <div className="w-full max-w-sm py-10">
        <div className="text-center mb-6">
          <h1 className="text-2xl font-bold text-foreground">绩效管理系统</h1>
          <p className="text-muted-foreground mt-2">欢迎回来</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>登录账户</CardTitle>
            <CardDescription>请输入您的邮箱和密码进行登录</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit}>
              <div className="flex flex-col gap-6">
                <div className="grid gap-3">
                  <Label htmlFor="email">邮箱</Label>
                  <Input
                    id="email"
                    name="email"
                    type="email"
                    placeholder="请输入邮箱"
                    value={formData.email}
                    onChange={handleChange}
                    required
                  />
                </div>
                <div className="grid gap-3">
                  <Label htmlFor="password">密码</Label>
                  <Input
                    id="password"
                    name="password"
                    type="password"
                    placeholder="请输入密码"
                    value={formData.password}
                    onChange={handleChange}
                    required
                  />
                </div>
                <div className="flex flex-col gap-3">
                  <Button type="submit" className="w-full" disabled={loading}>
                    {loading ? "登录中..." : "登录"}
                  </Button>
                </div>
              </div>
              <div className="mt-4 text-center text-sm">
                还没有账户？{" "}
                <Link href="/auth/register" className="underline underline-offset-4 hover:text-primary">
                  立即注册
                </Link>
              </div>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
