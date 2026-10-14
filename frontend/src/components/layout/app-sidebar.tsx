"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { LogoMarkSVG, LogoSVG } from "@/components/icons/logo"
import { CommandPaletteTrigger } from "@/components/layout/command-palette"
import { UserMenu } from "@/components/layout/user-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { cn } from "@/lib/utils"
import {
  dashboardHome,
  isNavLinkActive,
  visibleSections,
  type NavLink,
  type Role,
} from "@/lib/navigation"
import type { User } from "@/schemas/user"

function NavMenuItem({ link, pathname }: { link: NavLink; pathname: string }) {
  const isActive = isNavLinkActive(pathname, link.url)

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        asChild
        isActive={isActive}
        tooltip={link.title}
        className="relative"
      >
        <Link href={link.url} aria-current={isActive ? "page" : undefined}>
          {/* The active state carries a shape as well as a colour, so it does not
              rely on telling two greys apart. Hidden in the icon rail, where it
              would sit on top of the icon instead of beside it. */}
          <span
            aria-hidden
            className={cn(
              "bg-sidebar-primary absolute inset-y-1 left-0 w-[3px] rounded-full transition-opacity group-data-[collapsible=icon]:hidden",
              isActive ? "opacity-100" : "opacity-0",
            )}
          />
          <link.icon />
          <span>{link.title}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

export function AppSidebar({
  role,
  user = null,
}: {
  role?: Role
  user?: User | null
}) {
  const pathname = usePathname()

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        {/* Both children are hidden in the icon rail: the logo is 60px wide and
            the rail is 48px, and the ⌘K hint has no room for its label. */}
        <div className="flex h-8 items-center justify-between gap-2 px-2 py-1 group-data-[collapsible=icon]:hidden">
          <Link
            href={dashboardHome.url}
            className="focus-visible:ring-ring/50 flex items-center gap-1.5 rounded-md focus-visible:ring-[3px] focus-visible:outline-none"
          >
            <LogoSVG className="fill-sidebar-foreground h-6 w-15" />
            <span aria-hidden className="text-muted-foreground text-xs">
              (beta)
            </span>
            <span className="sr-only">M80 (beta) — ir para o dashboard</span>
          </Link>
          <CommandPaletteTrigger />
        </div>
        {/* The rail still needs a brand mark, otherwise collapsing the sidebar
            leaves no M80 anywhere on the screen. */}
        <Link
          href={dashboardHome.url}
          className="text-sidebar-foreground focus-visible:ring-ring/50 hidden size-8 items-center justify-center rounded-md group-data-[collapsible=icon]:flex focus-visible:ring-[3px] focus-visible:outline-none"
        >
          <LogoMarkSVG className="size-7" />
          <span className="sr-only">M80 (beta) — ir para o dashboard</span>
        </Link>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <NavMenuItem link={dashboardHome} pathname={pathname} />
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        {visibleSections(role).map((section) => (
          <SidebarGroup key={section.label}>
            <SidebarGroupLabel>{section.label}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {section.items.map((link) => (
                  <NavMenuItem key={link.url} link={link} pathname={pathname} />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>

      <SidebarFooter>
        <UserMenu user={user} />
      </SidebarFooter>
    </Sidebar>
  )
}
