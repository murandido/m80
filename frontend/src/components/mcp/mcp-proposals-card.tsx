"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Check, X } from "lucide-react"
import { toast } from "sonner"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { decideMCPProposalAction } from "@/actions/mcp"
import { formatDateTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { MCPProposal } from "@/schemas/mcp"

// What each proposal tool does, in the words the reviewer reads. The tool name stays visible
// next to it for whoever needs to look it up.
const TOOL_LABELS: Record<string, string> = {
  propose_cancel_issuance: "Cancelar emissão",
  propose_uncancel_issuance: "Reverter cancelamento",
  propose_delete_issuance: "Excluir emissão",
  propose_delete_adjustment: "Excluir alteração",
  propose_delete_point_batch: "Excluir lote de pontos",
  propose_batch_settlement: "Acerto de lote",
  propose_close_period: "Fechar período de estoque",
  propose_reopen_period: "Reabrir período de estoque",
}

export function toolLabel(tool: string): string {
  return TOOL_LABELS[tool] ?? tool
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  if (value == null) return null
  return (
    <details className="rounded-md border bg-slate-50 text-xs">
      <summary className="cursor-pointer px-3 py-2 font-medium text-slate-700">{label}</summary>
      <pre className="max-h-80 overflow-auto px-3 pb-3 whitespace-pre-wrap break-all">
        {JSON.stringify(value, null, 2)}
      </pre>
    </details>
  )
}

export function MCPProposalsCard({ proposals }: { proposals: MCPProposal[] }) {
  const router = useRouter()
  const [, startTransition] = useTransition()
  const [savingIds, setSavingIds] = useState<Set<string>>(new Set())

  function markSaving(id: string, saving: boolean) {
    setSavingIds((prev) => {
      const next = new Set(prev)
      if (saving) next.add(id)
      else next.delete(id)
      return next
    })
  }

  function decide(proposal: MCPProposal, decision: "approve" | "reject") {
    markSaving(proposal.id, true)
    startTransition(async () => {
      const result = await decideMCPProposalAction(proposal.id, decision)
      markSaving(proposal.id, false)

      if ("error" in result) {
        toast.error(result.error)
      } else if (result.data.status === "failed") {
        toast.error("A operação foi recusada pela API. Veja o resultado no histórico.")
      } else if (decision === "approve") {
        toast.success("Proposta aprovada e aplicada.")
      } else {
        toast.success("Proposta rejeitada.")
      }
      router.refresh()
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Propostas pendentes</CardTitle>
        <CardDescription>
          Operações que o agente não pode fazer sozinho. Ao aprovar, a operação roda com a sua
          sessão, como se você clicasse o botão correspondente. Expiram em 48h.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {proposals.length === 0 && (
          <p className="py-6 text-center text-sm text-muted-foreground">Nenhuma proposta pendente.</p>
        )}
        {proposals.map((proposal) => {
          const isSaving = savingIds.has(proposal.id)
          const operation = proposal.operations?.[0]

          return (
            <div
              key={proposal.id}
              className={cn("flex flex-col gap-3 rounded-md border p-4", isSaving && "opacity-60")}
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex flex-col gap-1">
                  <div className="flex items-center gap-2">
                    <Badge variant="secondary">{toolLabel(proposal.tool)}</Badge>
                    <span className="text-xs text-muted-foreground">
                      {proposal.user_name ? `por ${proposal.user_name} · ` : ""}
                      {formatDateTime(proposal.created_at)} · expira {formatDateTime(proposal.expires_at)}
                    </span>
                  </div>
                  <p className="text-sm text-slate-800">{proposal.summary}</p>
                  {operation && (
                    <code className="text-xs text-muted-foreground">
                      {operation.method} /api/v1{operation.path}
                    </code>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={isSaving}
                    onClick={() => decide(proposal, "reject")}
                  >
                    <X className="h-4 w-4" />
                    Rejeitar
                  </Button>
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button type="button" size="sm" disabled={isSaving}>
                        <Check className="h-4 w-4" />
                        Aprovar
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>Aprovar: {toolLabel(proposal.tool)}?</AlertDialogTitle>
                        <AlertDialogDescription>{proposal.summary}</AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Voltar</AlertDialogCancel>
                        <AlertDialogAction onClick={() => decide(proposal, "approve")}>
                          Aprovar e aplicar
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                </div>
              </div>
              <JsonBlock label="Estado atual (no momento da proposta)" value={proposal.preview} />
              <JsonBlock label="Dados enviados" value={operation?.body} />
            </div>
          )
        })}
      </CardContent>
    </Card>
  )
}

const STATUS_LABELS: Record<MCPProposal["status"], string> = {
  pending: "Pendente",
  applied: "Aplicada",
  rejected: "Rejeitada",
  failed: "Falhou",
  expired: "Expirada",
}

export function MCPProposalHistory({ proposals }: { proposals: MCPProposal[] }) {
  const decided = proposals.filter((proposal) => proposal.status !== "pending")

  return (
    <Card>
      <CardHeader>
        <CardTitle>Propostas decididas</CardTitle>
        <CardDescription>As últimas propostas aprovadas, rejeitadas, que falharam ou expiraram.</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {decided.length === 0 && (
          <p className="py-6 text-center text-sm text-muted-foreground">Nada decidido ainda.</p>
        )}
        {decided.map((proposal) => (
          <div key={proposal.id} className="flex flex-col gap-2 rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <Badge variant={proposal.status === "applied" ? "default" : "secondary"}>
                {STATUS_LABELS[proposal.status]}
              </Badge>
              <span className="font-medium">{toolLabel(proposal.tool)}</span>
              <span className="text-xs text-muted-foreground">
                {proposal.decided_at ? formatDateTime(proposal.decided_at) : ""}
                {proposal.decided_by_name ? ` · ${proposal.decided_by_name}` : ""}
              </span>
            </div>
            <p className="text-sm text-slate-700">{proposal.summary}</p>
            <JsonBlock label="Resultado" value={proposal.result} />
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
