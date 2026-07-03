import { LogoSVG } from "@/components/icons/logo"
import { SidebarTrigger } from "@/components/ui/sidebar"

export function Header() {
  return (
    <header className="flex flex-none items-center justify-between bg-slate-700 px-6 py-4 sm:px-12">
      <LogoSVG className="h-6 w-15 fill-white" />
      <SidebarTrigger className="text-white hover:bg-slate-600 hover:text-white" />
    </header>
  )
}
