import { AppSidebar } from "@/components/layout/app-sidebar"
import { Header } from "@/components/layout/header"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"

export function DashboardShell({ children }: { children: React.ReactNode }) {
  return (
    <SidebarProvider>
      <SidebarInset>
        <Header />
        <main className="flex-1 overflow-auto">{children}</main>
      </SidebarInset>
      <AppSidebar />
    </SidebarProvider>
  )
}
