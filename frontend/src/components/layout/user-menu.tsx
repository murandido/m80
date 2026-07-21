"use client"

import Link from "next/link"
import { LogOut } from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Badge } from "@/components/ui/badge"
import type { User } from "@/schemas/user"

const roleLabels: Record<User["role"], string> = {
  admin: "Administrador",
  user: "Usuário",
}

export function UserMenu({ user }: { user: User | null }) {
  if (!user) {
    return null
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Badge
          asChild
          variant="secondary"
          className="cursor-pointer border border-white/10 bg-white/10 text-white hover:bg-white/20"
        >
          <button type="button">
            <span className="max-w-[140px] truncate">{user.name}</span>
          </button>
        </Badge>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel className="font-normal">
          <span className="block truncate font-medium">{user.name}</span>
          <span className="block text-xs text-muted-foreground">
            {roleLabels[user.role]}
          </span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" asChild>
          <Link href="/api/logout">
            <LogOut />
            Sair
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
