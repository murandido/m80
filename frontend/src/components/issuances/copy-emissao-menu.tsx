"use client"

import { Calculator, Copy, MessageSquareText } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { copyToClipboard } from "@/lib/clipboard"
import { useGridContext } from "@/components/issuances/grid-context"
import {
  buildCalculo,
  buildEmissaoResumo,
  EmissaoText,
} from "@/components/issuances/emissao-message"
import { IssuanceRow } from "@/components/issuances/types"

// Copies either the whole EMISSAO message or just its calculation line. Refusing to copy a
// half-built message (and naming the empty columns) beats handing over text the bot would
// reject at the other end, where the operator has already pasted and moved on.
export async function copyEmissaoText(result: EmissaoText, what: "resumo" | "cálculo") {
  if ("missing" in result) {
    toast.error(`Faltam dados para montar o ${what}: ${result.missing.join(", ")}`)
    return
  }
  await copyToClipboard(result.text, `${what === "resumo" ? "Resumo" : "Cálculo"} copiado para a área de transferência`)
}

export function CopyEmissaoMenu({ row }: { row: IssuanceRow }) {
  // The context's reference lists are the merged ones - they include accounts and agencies
  // created during this session, which a row can already be pointing at.
  const ref = useGridContext()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          title="Copiar a emissão"
          className="h-6 w-6 opacity-0 transition-opacity group-hover/row:opacity-100"
        >
          <Copy className="h-3.5 w-3.5 text-slate-500" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          onClick={() => void copyEmissaoText(buildEmissaoResumo(row, ref), "resumo")}
        >
          <MessageSquareText />
          Copiar resumo
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => void copyEmissaoText(buildCalculo(row), "cálculo")}>
          <Calculator />
          Copiar cálculo
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
