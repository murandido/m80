import { AppSidebar } from "@/components/layout/app-sidebar"
import { Header } from "@/components/layout/header"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import type { SystemStatus } from "@/schemas/status"
import type { User } from "@/schemas/user"

export function DashboardShell({
  children,
  isAdmin = false,
  user = null,
  systemStatus = null,
}: {
  children: React.ReactNode
  isAdmin?: boolean
  user?: User | null
  systemStatus?: SystemStatus | null
}) {
  return (
    <SidebarProvider>
      <SidebarInset>
        <Header user={user} systemStatus={systemStatus} isAdmin={isAdmin} />
        <main className="flex-1 overflow-auto">{children}</main>
      </SidebarInset>
      <AppSidebar isAdmin={isAdmin} />
    </SidebarProvider>
  )
}
