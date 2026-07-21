import { LogoSVG } from "@/components/icons/logo"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { UserMenu } from "@/components/layout/user-menu"
import type { User } from "@/schemas/user"

export function Header({ user }: { user: User | null }) {
  return (
    <header className="flex flex-none items-center justify-between bg-slate-700 px-6 py-4 sm:px-12">
      <LogoSVG className="h-6 w-15 fill-white" />
      <div className="flex items-center gap-2">
        <UserMenu user={user} />
        <SidebarTrigger className="text-white hover:bg-slate-600 hover:text-white" />
      </div>
    </header>
  )
}
