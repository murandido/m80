import { LogoSVG } from "@/components/icons/logo"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { UserMenu } from "@/components/layout/user-menu"
import { SystemStatusIndicator } from "@/components/layout/system-status-indicator"
import type { SystemStatus } from "@/schemas/status"
import type { User } from "@/schemas/user"

export function Header({
  user,
  systemStatus = null,
  isAdmin = false,
}: {
  user: User | null
  systemStatus?: SystemStatus | null
  isAdmin?: boolean
}) {
  return (
    <header className="flex flex-none items-center justify-between bg-slate-700 px-6 py-4 sm:px-12">
      <LogoSVG className="h-6 w-15 fill-white" />
      <div className="flex items-center gap-2">
        {/* In the header rather than on the dashboard home so that it reaches
            the people who never leave the issuances grid. */}
        <SystemStatusIndicator initialStatus={systemStatus} isAdmin={isAdmin} />
        <UserMenu user={user} />
        <SidebarTrigger className="text-white hover:bg-slate-600 hover:text-white" />
      </div>
    </header>
  )
}
