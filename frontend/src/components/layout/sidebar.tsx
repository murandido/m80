export function SidebarBase() {
  return (
    <div className="size-full">
      <div className="text-cyan-400">M-80</div>
    </div>
  )
}

export function MobileSidebar() {
  return (
    <div className="fixed size-full bg-slate-800 lg:hidden">
      <div>
        <SidebarBase />
      </div>
    </div>
  )
}

export function DesktopSidebar() {
  return (
    <div className="hidden lg:block">
      <div></div>
    </div>
  )
}
