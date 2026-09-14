import type { Metadata } from "next"
import { Inter } from "next/font/google"
import { headers } from "next/headers"
import { AppProvider } from "@/lib/app-context"

import { AuthProvider } from "@/lib/auth-context"
import { ThemeProvider } from "@/lib/theme-context"
import { DootaskProvider } from "@/lib/dootask-context"
import { LayoutProvider } from "@/lib/layout-context"
import { UnreadProvider } from "@/lib/unread-context"
import { NotificationProvider } from "@/lib/notification-context"
import "./globals.css"

const inter = Inter({ subsets: ["latin"] })

// 将metadata移到单独的组件中，因为这是客户端组件
const metadata: Metadata = {
  title: "KPI绩效考核系统",
  description: "基于NextJS和React的KPI绩效考核管理系统",
}

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const headersList = await headers()
  const theme = headersList.get("x-theme") || undefined

  return (
    <html lang="zh-CN" className={theme} data-theme={theme} disable-auto-theme={theme}>
      <head>
        <title>{String(metadata.title)}</title>
        <meta name="description" content={String(metadata.description)} />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
      </head>
      <body className={inter.className}>
        <ThemeProvider>
          <AppProvider>
            <DootaskProvider>
              <AuthProvider>
                <NotificationProvider>
                  <UnreadProvider>
                    <LayoutProvider>{children}</LayoutProvider>
                  </UnreadProvider>
                </NotificationProvider>
              </AuthProvider>
            </DootaskProvider>
          </AppProvider>
        </ThemeProvider>
      </body>
    </html>
  )
}
