import { NavBreadcrumb } from "@/components/layout/nav-breadcrumb"
import { SystemStatusIndicator } from "@/components/layout/system-status-indicator"
import { Separator } from "@/components/ui/separator"
import { SidebarTrigger } from "@/components/ui/sidebar"
import type { SystemStatus } from "@/schemas/status"

export function Header({
  systemStatus = null,
  isAdmin = false,
}: {
  systemStatus?: SystemStatus | null
  isAdmin?: boolean
}) {
  return (
    <header className="sticky top-0 z-10 flex h-12 flex-none items-center gap-2 border-b bg-background px-3">
      <SidebarTrigger className="-ml-1" />
      <Separator orientation="vertical" className="mr-1 data-[orientation=vertical]:h-4" />
      <NavBreadcrumb />
      {/* In the header rather than on the dashboard home so that it reaches
          the people who never leave the issuances grid. */}
      <div className="ml-auto flex items-center gap-2">
        <SystemStatusIndicator initialStatus={systemStatus} isAdmin={isAdmin} />
      </div>
    </header>
  )
}
