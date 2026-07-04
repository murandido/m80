"use client"

import { Copy, KeyRound, User } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

interface CopyCredentialsMenuProps {
  login: string | null
  password: string | null
}

async function copy(value: string, successMessage: string) {
  try {
    await navigator.clipboard.writeText(value)
    toast.success(successMessage)
  } catch {
    toast.error("Não foi possível copiar para a área de transferência.")
  }
}

export function CopyCredentialsMenu({ login, password }: CopyCredentialsMenuProps) {
  if (!login && !password) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="icon"
          className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-slate-600 hover:bg-slate-600 hover:text-white active:border-slate-900 active:bg-slate-900 active:text-white"
        >
          <Copy className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          disabled={!login}
          onClick={() => login && copy(login, "Login copiado para a área de transferência")}
        >
          <User />
          Copiar login
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={!password}
          onClick={() =>
            password && copy(password, "Senha copiada para a área de transferência")
          }
        >
          <KeyRound />
          Copiar senha
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
