"use client"

import { useState } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { ChevronRight, Contact, LayoutDashboard, Users } from "lucide-react"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from "@/components/ui/sidebar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

type IconComponent = React.ComponentType<{ className?: string }>

interface NavLink {
  title: string
  url: string
  icon: IconComponent
}

interface NavGroup {
  title: string
  icon: IconComponent
  items: { title: string; url: string; icon: IconComponent }[]
}

type NavEntry = NavLink | NavGroup

function isNavGroup(item: NavEntry): item is NavGroup {
  return "items" in item
}

// Add a new dashboard page by adding an entry here — no other file needs to change.
// A plain entry ({ title, url, icon }) renders as a direct link. An entry with
// `items` renders as a collapsible group whose children are links (e.g. "Contas e
// Pessoas" today only holds "Pessoas" — add "Contas" to `items` once its CRUD exists).
const navItems: NavEntry[] = [
  { title: "Dashboard", url: "/dashboard", icon: LayoutDashboard },
  { title: "Usuários", url: "/dashboard/users", icon: Users },
  {
    title: "Contas e Pessoas",
    icon: Contact,
    items: [{ title: "Pessoas", url: "/dashboard/people", icon: Users }],
  },
]

export function AppSidebar() {
  const pathname = usePathname()
  const { state, isMobile } = useSidebar()
  // The mobile sidebar always renders full-width inside a Sheet, regardless of
  // `state` — so the icon-only rail only exists when collapsed on desktop.
  const isIconMode = state === "collapsed" && !isMobile

  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>(() => {
    const initial: Record<string, boolean> = {}
    for (const item of navItems) {
      if (isNavGroup(item)) {
        initial[item.title] = item.items.some((child) =>
          pathname.startsWith(child.url),
        )
      }
    }
    return initial
  })

  function toggleGroup(title: string) {
    setOpenGroups((prev) => ({ ...prev, [title]: !prev[title] }))
  }

  return (
    <Sidebar side="right" collapsible="icon">
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Navegação</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map((item) => {
                if (isNavGroup(item)) {
                  const isChildActive = item.items.some((child) =>
                    pathname.startsWith(child.url),
                  )

                  // Icon-only rail: the inline sub-menu is hidden by CSS in this
                  // mode, so surface the children as a dropdown instead — otherwise
                  // there'd be no way to reach "Pessoas" without expanding the sidebar.
                  if (isIconMode) {
                    return (
                      <SidebarMenuItem key={item.title}>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <SidebarMenuButton isActive={isChildActive}>
                              <item.icon />
                              <span>{item.title}</span>
                            </SidebarMenuButton>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent side="left" align="start">
                            {item.items.map((child) => (
                              <DropdownMenuItem key={child.url} asChild>
                                <Link href={child.url}>
                                  <child.icon className="h-4 w-4" />
                                  <span>{child.title}</span>
                                </Link>
                              </DropdownMenuItem>
                            ))}
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </SidebarMenuItem>
                    )
                  }

                  const isOpen = openGroups[item.title]

                  return (
                    <SidebarMenuItem key={item.title}>
                      <SidebarMenuButton
                        isActive={isChildActive}
                        tooltip={item.title}
                        onClick={() => toggleGroup(item.title)}
                      >
                        <item.icon />
                        <span>{item.title}</span>
                        <ChevronRight
                          className={cn(
                            "ml-auto h-4 w-4 transition-transform duration-200",
                            isOpen && "rotate-90",
                          )}
                        />
                      </SidebarMenuButton>
                      {isOpen && (
                        <SidebarMenuSub>
                          {item.items.map((child) => (
                            <SidebarMenuSubItem key={child.url}>
                              <SidebarMenuSubButton
                                asChild
                                isActive={pathname.startsWith(child.url)}
                              >
                                <Link href={child.url}>
                                  <span>{child.title}</span>
                                </Link>
                              </SidebarMenuSubButton>
                            </SidebarMenuSubItem>
                          ))}
                        </SidebarMenuSub>
                      )}
                    </SidebarMenuItem>
                  )
                }

                const isActive =
                  item.url === "/dashboard"
                    ? pathname === item.url
                    : pathname.startsWith(item.url)

                return (
                  <SidebarMenuItem key={item.url}>
                    <SidebarMenuButton
                      asChild
                      isActive={isActive}
                      tooltip={item.title}
                    >
                      <Link href={item.url}>
                        <item.icon />
                        <span>{item.title}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  )
}
