"use client"

import { createContext, useContext, useEffect, useState } from "react"
import { storage } from "./storage"

type Theme = "light" | "dark" | "system"
type ActualTheme = "light" | "dark"

interface ThemeContextType {
  theme: Theme
  setTheme: (theme: Theme) => void
  actualTheme: ActualTheme
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined)

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setTheme] = useState<Theme>("system")
  const [actualTheme, setActualTheme] = useState<ActualTheme>("light")

  // 获取系统主题
  const getSystemTheme = (): ActualTheme => {
    if (typeof window !== "undefined") {
      return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
    }
    return "light"
  }

  // 计算实际主题
  const calculateActualTheme = (currentTheme: Theme): ActualTheme => {
    if (currentTheme === "system") {
      return getSystemTheme()
    }
    return currentTheme as ActualTheme
  }

  // 应用主题到DOM
  const applyTheme = (newActualTheme: ActualTheme) => {
    const root = document.documentElement
    if (root.getAttribute("disable-auto-theme")) {
      return
    }
    root.classList.remove("light", "dark")
    root.classList.add(newActualTheme)
    root.setAttribute("data-theme", newActualTheme)
    setActualTheme(newActualTheme)
  }

  // 初始化主题
  useEffect(() => {
    // 从缓存中获取保存的主题设置
    const savedTheme = storage.getItem("theme") as Theme
    if (savedTheme && ["light", "dark", "system"].includes(savedTheme)) {
      setTheme(savedTheme)
      const newActualTheme = calculateActualTheme(savedTheme)
      applyTheme(newActualTheme)
    } else {
      // 默认跟随系统
      const systemTheme = getSystemTheme()
      applyTheme(systemTheme)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 监听系统主题变化
  useEffect(() => {
    if (typeof window === "undefined") return

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)")
    const handleChange = (e: MediaQueryListEvent) => {
      if (theme === "system") {
        applyTheme(e.matches ? "dark" : "light")
      }
    }

    mediaQuery.addEventListener("change", handleChange)
    return () => mediaQuery.removeEventListener("change", handleChange)
  }, [theme])

  // 主题切换函数
  const handleSetTheme = (newTheme: Theme) => {
    setTheme(newTheme)
    storage.setItem("theme", newTheme)
    const newActualTheme = calculateActualTheme(newTheme)
    applyTheme(newActualTheme)
  }

  const value: ThemeContextType = {
    theme,
    setTheme: handleSetTheme,
    actualTheme,
  }

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme() {
  const context = useContext(ThemeContext)
  if (context === undefined) {
    throw new Error("useTheme must be used within a ThemeProvider")
  }
  return context
}
