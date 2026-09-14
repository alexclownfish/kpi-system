"use client"

import { createContext, useState, ReactNode, useContext, useEffect, useMemo } from "react"
import { useAuth } from "./auth-context"
import { useNotification } from "./notification-context"
import { evaluationApi, invitationApi } from "./api"
import { debounce } from "lodash"

interface UnreadContextType {
  // 待确认评估数量
  unreadEvaluations: number
  refreshUnreadEvaluations: () => void

  // 待确认邀请数量
  unreadInvitations: number
  refreshUnreadInvitations: () => void
}

const UnreadContext = createContext<UnreadContextType | undefined>(undefined)

export function UnreadProvider({ children }: { children: ReactNode }) {
  const { userId } = useAuth()
  const { onMessage } = useNotification()
  const [unreadEvaluations, setUnreadEvaluations] = useState(0)
  const [unreadInvitations, setUnreadInvitations] = useState(0)

  // 待确认评估数量
  const refreshUnreadEvaluations = useMemo(
    () =>
      debounce(async () => {
        if (userId) {
          try {
            const response = await evaluationApi.getPendingCount()
            setUnreadEvaluations(response.count)
          } catch (error) {
            console.error("获取待确认评估数量失败:", error)
            setUnreadEvaluations(0)
          }
        } else {
          setUnreadEvaluations(0)
        }
      }, 500),
    [userId]
  )

  // 待确认邀请数量
  const refreshUnreadInvitations = useMemo(
    () =>
      debounce(async () => {
        if (userId) {
          try {
            const response = await invitationApi.getPendingCount()
            setUnreadInvitations(response.count)
          } catch (error) {
            console.error("获取待确认邀请数量失败:", error)
            setUnreadInvitations(0)
          }
        } else {
          setUnreadInvitations(0)
        }
      }, 500),
    [userId]
  )

  // 监听实时通知消息
  useEffect(() => {
    const unsubscribe = onMessage(message => {
      if (message.type === "connected" || message.type === "heartbeat") {
        return
      }

      // 根据消息类型更新相应的数量
      const messageType = message.type

      if (messageType.includes("evaluation")) {
        // 评估相关消息，刷新评估数量
        refreshUnreadEvaluations()
      }

      if (messageType.includes("invitation")) {
        // 邀请相关消息，刷新邀请数量
        refreshUnreadInvitations()
      }
    })

    return unsubscribe
  }, [onMessage, refreshUnreadEvaluations, refreshUnreadInvitations])

  useEffect(() => {
    refreshUnreadEvaluations()
    refreshUnreadInvitations()
  }, [refreshUnreadEvaluations, refreshUnreadInvitations])

  return (
    <UnreadContext.Provider
      value={{
        unreadEvaluations,
        refreshUnreadEvaluations,
        unreadInvitations,
        refreshUnreadInvitations,
      }}
    >
      {children}
    </UnreadContext.Provider>
  )
}

export function useUnreadContext() {
  const context = useContext(UnreadContext)
  if (context === undefined) {
    throw new Error("useUnreadContext must be used within a UnreadProvider")
  }
  return context
}
