import { AppSidebar } from "@/components/layout/app-sidebar"
import { Header } from "@/components/layout/header"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import type { User } from "@/schemas/user"

export function DashboardShell({
  children,
  isAdmin = false,
  user = null,
}: {
  children: React.ReactNode
  isAdmin?: boolean
  user?: User | null
}) {
  return (
    <SidebarProvider>
      <SidebarInset>
        <Header user={user} />
        <main className="flex-1 overflow-auto">{children}</main>
      </SidebarInset>
      <AppSidebar isAdmin={isAdmin} />
    </SidebarProvider>
  )
}
