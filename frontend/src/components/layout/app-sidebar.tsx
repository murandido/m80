"use client"

import { useState } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import {
  Award,
  ChevronRight,
  Contact,
  Building2,
  Landmark,
  LayoutDashboard,
  Plane,
  Ticket,
  Users,
} from "lucide-react"
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
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

type IconComponent = React.ComponentType<{ className?: string }>

interface NavLink {
  title: string
  url: string
  icon: IconComponent
}

interface NavGroupItem {
  title: string
  url: string
  icon: IconComponent
  // Rendered greyed-out and non-navigating — for features without a page yet.
  disabled?: boolean
}

interface NavGroup {
  title: string
  icon: IconComponent
  items: NavGroupItem[]
}

type NavEntry = NavLink | NavGroup

function isNavGroup(item: NavEntry): item is NavGroup {
  return "items" in item
}

// Add a new dashboard page by adding an entry here — no other file needs to change.
// A plain entry ({ title, url, icon }) renders as a direct link. An entry with
// `items` renders as a collapsible group whose children are links.
const navItems: NavEntry[] = [
  { title: "Dashboard", url: "/dashboard", icon: LayoutDashboard },
  { title: "Usuários", url: "/dashboard/users", icon: Users },
  { title: "Agências", url: "/dashboard/agencies", icon: Building2 },
  { title: "Emissões", url: "/dashboard/issuances", icon: Ticket },
  {
    title: "Contas e Pessoas",
    icon: Contact,
    items: [
      { title: "Contas", url: "/dashboard/accounts", icon: Landmark },
      { title: "Pessoas", url: "/dashboard/people", icon: Users },
    ],
  },
  {
    title: "Programas de Fidelidade e CIAs Aéreas",
    icon: Plane,
    items: [
      { title: "Programas de Fidelidade", url: "/dashboard/loyalty-programs", icon: Award },
      { title: "CIAs Aéreas", url: "/dashboard/airlines", icon: Plane },
    ],
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
        initial[item.title] = item.items.some(
          (child) => !child.disabled && pathname.startsWith(child.url),
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
                  const isChildActive = item.items.some(
                    (child) => !child.disabled && pathname.startsWith(child.url),
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
                            {item.items.map((child) =>
                              child.disabled ? (
                                <DropdownMenuItem key={child.title} disabled>
                                  <child.icon className="h-4 w-4" />
                                  <span>{child.title}</span>
                                </DropdownMenuItem>
                              ) : (
                                <DropdownMenuItem key={child.title} asChild>
                                  <Link href={child.url}>
                                    <child.icon className="h-4 w-4" />
                                    <span>{child.title}</span>
                                  </Link>
                                </DropdownMenuItem>
                              ),
                            )}
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
                        className="h-auto items-start py-2"
                      >
                        <item.icon className="mt-0.5 shrink-0" />
                        <span className="min-w-0 flex-1">{item.title}</span>
                        <ChevronRight
                          className={cn(
                            "ml-auto mt-0.5 h-4 w-4 shrink-0 transition-transform duration-200",
                            isOpen && "rotate-90",
                          )}
                        />
                      </SidebarMenuButton>
                      {isOpen && (
                        <SidebarMenuSub>
                          {item.items.map((child) =>
                            child.disabled ? (
                              <SidebarMenuSubItem key={child.title}>
                                <SidebarMenuSubButton
                                  aria-disabled="true"
                                  className="h-auto items-start py-1.5"
                                >
                                  <span className="min-w-0 flex-1">{child.title}</span>
                                  <Badge
                                    variant="outline"
                                    className="ml-auto mt-0.5 shrink-0 text-[10px]"
                                  >
                                    Em breve
                                  </Badge>
                                </SidebarMenuSubButton>
                              </SidebarMenuSubItem>
                            ) : (
                              <SidebarMenuSubItem key={child.title}>
                                <SidebarMenuSubButton
                                  asChild
                                  isActive={pathname.startsWith(child.url)}
                                >
                                  <Link href={child.url}>
                                    <span>{child.title}</span>
                                  </Link>
                                </SidebarMenuSubButton>
                              </SidebarMenuSubItem>
                            ),
                          )}
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
