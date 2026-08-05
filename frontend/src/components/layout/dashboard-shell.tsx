import { AppSidebar } from "@/components/layout/app-sidebar"
import { CommandPaletteProvider } from "@/components/layout/command-palette"
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
    <CommandPaletteProvider isAdmin={isAdmin}>
      <SidebarProvider>
        <a
          href="#conteudo"
          className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:text-sm focus:ring-[3px] focus:ring-ring/50"
        >
          Pular para o conteúdo
        </a>
        <AppSidebar isAdmin={isAdmin} user={user} />
        {/* SidebarInset already renders the <main>; the inner wrapper is the
            scroll container and the skip link's target. */}
        <SidebarInset>
          <Header systemStatus={systemStatus} isAdmin={isAdmin} />
          <div id="conteudo" tabIndex={-1} className="flex-1 overflow-auto outline-none">
            {children}
          </div>
        </SidebarInset>
      </SidebarProvider>
    </CommandPaletteProvider>
  )
}
