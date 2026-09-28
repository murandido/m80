"use client"

import Link from "next/link"
import { ChevronsUpDown, LogOut, UserRound } from "lucide-react"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar"
import { type User, userRoleLabels } from "@/schemas/user"

function initials(name: string): string {
  const parts = name.trim().split(/\s+/)
  const first = parts.at(0)?.charAt(0) ?? ""
  const last = parts.length > 1 ? (parts.at(-1)?.charAt(0) ?? "") : ""
  return (first + last).toUpperCase()
}

export function UserMenu({ user }: { user: User | null }) {
  const { isMobile } = useSidebar()

  if (!user) {
    return null
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" aria-label={`Conta de ${user.name}`}>
              <Avatar className="size-8 rounded-md">
                <AvatarFallback className="rounded-md bg-sidebar-primary text-xs text-sidebar-primary-foreground">
                  {initials(user.name)}
                </AvatarFallback>
              </Avatar>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">{user.name}</span>
                <span className="truncate text-xs text-sidebar-foreground/70">
                  {userRoleLabels[user.role]}
                </span>
              </div>
              <ChevronsUpDown className="ml-auto size-4" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-56"
            side={isMobile ? "bottom" : "right"}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="font-normal">
              <span className="block truncate font-medium">{user.name}</span>
              <span className="block text-xs text-muted-foreground">
                {userRoleLabels[user.role]}
              </span>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/dashboard/profile">
                <UserRound />
                Meu perfil
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem variant="destructive" asChild>
              <Link href="/api/logout">
                <LogOut />
                Sair
              </Link>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
