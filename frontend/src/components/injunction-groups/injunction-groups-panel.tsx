"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
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
import { InputGroup, InputGroupInput } from "@/components/ui/input-group"
import { InjunctionGroupFormPanel } from "@/components/injunction-groups/injunction-group-form"
import {
  checkInjunctionGroupAction,
  deleteInjunctionGroupAction,
  importInjunctionPeriodAction,
} from "@/actions/injunction-group"
import type { CheckItem, CheckResult, InjunctionAccountOption, InjunctionGroup } from "@/schemas/injunction-group"

const BUCKETS: { key: keyof CheckResult; label: string; tone: "ok" | "warn" | "bad" }[] = [
  { key: "missing_in_panel_confirmed", label: "Faltam no painel (planilha e extrato concordam)", tone: "ok" },
  { key: "missing_in_panel_unconfirmed", label: "Faltam no painel (só a planilha)", tone: "warn" },
  { key: "differs_from_panel", label: "Diferentes do painel", tone: "bad" },
  { key: "missing_in_sheet_and_panel", label: "Só no extrato", tone: "bad" },
  { key: "pending_cancellations", label: "Estornos a aplicar", tone: "warn" },
  { key: "invalid", label: "Linhas incompletas", tone: "warn" },
  { key: "needs_mapping", label: "Sem mapeamento", tone: "warn" },
]

function isoDay(date: Date): string {
  return date.toISOString().slice(0, 10)
}

function CheckView({ result }: { result: CheckResult }) {
  return (
    <div className="flex flex-col gap-2 text-sm">
      <p className="text-xs text-muted-foreground">
        {result.from} a {result.to} · {result.sheet_rows} linhas das contas da Tropa
        {result.statements.map((s) => (
          <span key={s.account_id}>
            {" "}
            · extrato {s.program}: {s.error ? s.error : `${s.from} a ${s.to}`}
          </span>
        ))}
      </p>
      {BUCKETS.map((bucket) => {
        const items = result[bucket.key] as CheckItem[]
        return (
          <details key={bucket.key} className="rounded-md border" open={items.length > 0 && bucket.tone === "bad"}>
            <summary className="flex cursor-pointer items-center gap-2 px-3 py-2">
              <Badge variant={items.length === 0 ? "secondary" : bucket.tone === "bad" ? "destructive" : "default"}>{items.length}</Badge>
              {bucket.label}
            </summary>
            {items.length > 0 && (
              <ul className="flex flex-col gap-1 border-t px-3 py-2 text-xs">
                {items.map((item) => (
                  <li key={`${item.day}-${item.block_index}-${item.number}-${item.locator}`}>
                    <code>{item.locator}</code> {item.day && `(${item.day})`} {item.program} {item.account_label} ·{" "}
                    {item.points.toLocaleString("pt-BR")} pts {item.issuer && `· ${item.issuer}`} — {item.detail}
                  </li>
                ))}
              </ul>
            )}
          </details>
        )
      })}
    </div>
  )
}

function GroupCard({
  group,
  options,
}: {
  group: InjunctionGroup
  options: { accounts: InjunctionAccountOption[]; users: { id: string; name: string }[]; agencies: { id: string; name: string }[] }
}) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  const [editing, setEditing] = useState(false)
  const [result, setResult] = useState<CheckResult | null>(null)
  const today = new Date()
  const [from, setFrom] = useState(isoDay(new Date(today.getTime() - 7 * 86400000)))
  const [to, setTo] = useState(isoDay(today))

  function check() {
    startTransition(async () => {
      const response = await checkInjunctionGroupAction(group.id, from, to)
      if ("error" in response || !response.data) {
        toast.error("error" in response ? response.error : "Erro ao conferir.")
        return
      }
      setResult(response.data)
    })
  }

  function importPeriod() {
    startTransition(async () => {
      const response = await importInjunctionPeriodAction(group.id, from, to)
      if ("error" in response || !response.data) {
        toast.error("error" in response ? response.error : "Erro ao importar.")
        return
      }
      const total = response.data.total
      toast.success(
        `Importadas ${total.imported}, canceladas ${total.cancelled + total.created_and_cancelled}, puladas ${total.skipped.length}.`,
      )
      router.refresh()
    })
  }

  function remove() {
    startTransition(async () => {
      const response = await deleteInjunctionGroupAction(group.id)
      if ("error" in response) {
        toast.error(response.error)
        return
      }
      router.refresh()
    })
  }

  if (editing) {
    return <InjunctionGroupFormPanel group={group} options={options} onDone={() => setEditing(false)} />
  }

  return (
    <div className="flex flex-col gap-3 rounded-md border p-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="flex flex-col gap-1 text-sm">
          <span className="font-medium">{group.name}</span>
          <span className="text-xs text-muted-foreground">
            {group.agency_name} · WhatsApp: {group.whatsapp_chat_name || "—"} · planilha:{" "}
            {group.has_spreadsheet ? group.spreadsheet_hint : "não cadastrada"}
          </span>
          <span className="text-xs">
            Contas: {group.accounts.map((a) => `${a.account_label} → ${a.program_code} ${a.nicknames}`).join("; ") || "nenhuma"}
          </span>
          <span className="text-xs">
            Emissores: {group.issuers.map((i) => `${i.issuer_name} → ${i.user_name}`).join("; ") || "nenhum"}
          </span>
        </div>
        <div className="flex gap-2">
          <Button type="button" variant="outline" size="sm" disabled={isPending} onClick={() => setEditing(true)}>
            Editar
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button type="button" variant="outline" size="sm" disabled={isPending}>
                Excluir
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Excluir {group.name}?</AlertDialogTitle>
                <AlertDialogDescription>Só o cadastro some; nenhuma emissão ou conta é alterada.</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Voltar</AlertDialogCancel>
                <AlertDialogAction onClick={remove}>Excluir</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <InputGroup className="w-40 bg-slate-200">
          <InputGroupInput type="date" value={from} onChange={(event) => setFrom(event.target.value)} aria-label="De" />
        </InputGroup>
        <InputGroup className="w-40 bg-slate-200">
          <InputGroupInput type="date" value={to} onChange={(event) => setTo(event.target.value)} aria-label="Até" />
        </InputGroup>
        <Button type="button" size="sm" disabled={isPending || !group.has_spreadsheet} onClick={check}>
          Conferir agora
        </Button>
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button type="button" variant="outline" size="sm" disabled={isPending || !group.has_spreadsheet}>
              Importar período
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                Importar {from} a {to}?
              </AlertDialogTitle>
              <AlertDialogDescription>
                Grava como emissões todas as linhas prontas das contas desta Tropa nesse período (e aplica os estornos).
                Linhas incompletas, sem mapeamento ou que já existem no painel ficam de fora. Rode &quot;Conferir agora&quot;
                antes para ver o que entra.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Voltar</AlertDialogCancel>
              <AlertDialogAction onClick={importPeriod}>Importar</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>

      {result && <CheckView result={result} />}
    </div>
  )
}

export function InjunctionGroupsPanel({
  groups,
  options,
}: {
  groups: InjunctionGroup[]
  options: { accounts: InjunctionAccountOption[]; users: { id: string; name: string }[]; agencies: { id: string; name: string }[] }
}) {
  const [creating, setCreating] = useState(false)
  return (
    <Card>
      <CardHeader>
        <CardTitle>Tropas</CardTitle>
        <CardDescription>
          Grupos de contas liminares que controlam as emissões numa planilha do OneDrive. Só as linhas das contas mapeadas
          são lidas; a taxa de resgate é ignorada.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {groups.length === 0 && !creating && <p className="text-sm text-muted-foreground">Nenhuma Tropa cadastrada.</p>}
        {groups.map((group) => (
          <GroupCard key={group.id} group={group} options={options} />
        ))}
        {creating ? (
          <InjunctionGroupFormPanel group={null} options={options} onDone={() => setCreating(false)} />
        ) : (
          <Button type="button" className="self-start" onClick={() => setCreating(true)}>
            Nova Tropa
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
