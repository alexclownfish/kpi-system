"use client"

import { cn } from "@/lib/utils"
import { Loader2 } from "lucide-react"
import { useEffect, useState } from "react"

interface LoadingProps {
  message?: string
  className?: string
  delay?: number
}

export default function Loading({ message = "正在加载...", className, delay = 0 }: LoadingProps) {
  const [show, setShow] = useState(false)

  useEffect(() => {
    if (delay === 0) {
      setShow(true)
      return
    }
    const timer = setTimeout(() => setShow(true), delay)
    return () => clearTimeout(timer)
  }, [delay])

  if (!show) return null

  return (
    <div className={cn("min-h-screen flex items-center justify-center bg-background", className)}>
      <div className="text-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 dark:border-blue-400 mx-auto mb-4"></div>
        <p className="text-muted-foreground">{message}</p>
      </div>
    </div>
  )
}

export function LoadingInline({ message, className, delay = 0 }: LoadingProps) {
  const [show, setShow] = useState(false)

  useEffect(() => {
    if (delay === 0) {
      setShow(true)
      return
    }
    const timer = setTimeout(() => setShow(true), delay)
    return () => clearTimeout(timer)
  }, [delay])

  if (!show) return null

  return (
    <div className={cn("flex items-center justify-center gap-2 text-muted-foreground", className)}>
      <Loader2 className="w-4 h-4 animate-spin" />
      {message && <div>{message}</div>}
    </div>
  )
}
