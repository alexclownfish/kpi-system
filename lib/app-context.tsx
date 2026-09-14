"use client"

import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react"
import AlertLayout, { AlertLayoutRef, AlertProps } from "@/components/alert"
import { Toaster } from "@/components/ui/sonner"
import { Badge } from "@/components/ui/badge"
import { CheckCircle, Clock, Eye, FileText, Star } from "lucide-react"

interface AppContextType {
  isTouch: boolean
  Alert: (title: string | AlertProps, message?: string) => Promise<void>
  Confirm: (title: string | AlertProps, message?: string) => Promise<boolean>

  getStatusBadge: (status: string) => React.ReactNode
}

const AppContext = createContext<AppContextType | undefined>(undefined)

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [isTouch, setIsTouch] = useState(false)
  const alertLayoutRef = useRef<AlertLayoutRef>(null)

  const Alert = useCallback((title: string | AlertProps, message?: string) => {
    return new Promise<void>(resolve => {
      alertLayoutRef.current?.setAlert({
        type: "alert",
        message,
        ...(typeof title === "string" ? { title } : title),

        onConfirm: () => {
          resolve()
        },
      })
    })
  }, [])

  const Confirm = useCallback((title: string | AlertProps, message?: string) => {
    return new Promise<boolean>(resolve => {
      alertLayoutRef.current?.setAlert({
        type: "confirm",
        message,
        ...(typeof title === "string" ? { title } : title),

        onConfirm: () => {
          resolve(true)
        },
        onClose: () => {
          resolve(false)
        },
      })
    })
  }, [])

  useEffect(() => {
    if (typeof window === 'undefined') {
      setIsTouch(false)
      return
    }
    setIsTouch('ontouchstart' in window || 'ontouchend' in window)
  }, [])

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "pending":
        return (
          <Badge variant="outline" className="text-yellow-600 border-yellow-600">
            <Clock className="w-3 h-3 mr-1" />
            待自评
          </Badge>
        )
      case "self_evaluated":
        return (
          <Badge variant="outline" className="text-blue-600 border-blue-600">
            <FileText className="w-3 h-3 mr-1" />
            待主管评估
          </Badge>
        )
      case "manager_evaluated":
        return (
          <Badge variant="outline" className="text-purple-600 border-purple-600">
            <Eye className="w-3 h-3 mr-1" />
            待HR审核
          </Badge>
        )
      case "pending_confirm":
        return (
          <Badge variant="outline" className="text-pink-600 border-pink-600">
            <Star className="w-3 h-3 mr-1" />
            待确认
          </Badge>
        )
      case "completed":
        return (
          <Badge variant="outline" className="text-green-600 border-green-600">
            <CheckCircle className="w-3 h-3 mr-1" />
            已完成
          </Badge>
        )
      default:
        return <Badge variant="outline">未知状态</Badge>
    }
  }

  return (
    <AppContext.Provider
      value={{
        isTouch,
        Alert,
        Confirm,
        getStatusBadge,
      }}
    >
      {children}

      {/* 弹出框 */}
      <AlertLayout ref={alertLayoutRef} />

      {/* 通知 */}
      <Toaster position="top-center" />
    </AppContext.Provider>
  )
}

export function useAppContext() {
  const context = useContext(AppContext)
  if (context === undefined) {
    throw new Error("useAppContext must be used within an AppProvider")
  }
  return context
}
