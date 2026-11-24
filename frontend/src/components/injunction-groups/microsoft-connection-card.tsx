"use client"

import { useTransition } from "react"
import { useRouter } from "next/navigation"
import { toast } from "sonner"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { disconnectMicrosoftAction, startMicrosoftConnectionAction } from "@/actions/injunction-group"
import { formatDateTime } from "@/lib/format"
import type { MicrosoftConnection } from "@/schemas/injunction-group"

const STATE_LABELS: Record<string, string> = {
  ok: "Conectada",
  needs_login: "Precisa conectar de novo",
  error: "Erro na última leitura",
}

// The Microsoft login the API reads the injunction groups' workbooks with. Read-only access to files
// shared with the account; one per installation.
export function MicrosoftConnectionCard({ connection }: { connection: MicrosoftConnection | null }) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  function connect() {
    startTransition(async () => {
      const result = await startMicrosoftConnectionAction()
      if ("error" in result || !result.data) {
        toast.error("error" in result ? result.error : "Erro ao iniciar o login Microsoft.")
        return
      }
      // Microsoft sends the browser back to /api/microsoft/callback, which lands here again.
      window.location.assign(result.data.authorize_url)
    })
  }

  function disconnect() {
    startTransition(async () => {
      const result = await disconnectMicrosoftAction()
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Conta Microsoft desconectada.")
      router.refresh()
    })
  }

  const connected = connection?.connected ?? false
  return (
    <Card>
      <CardHeader>
        <CardTitle>Conexão Microsoft</CardTitle>
        <CardDescription>
          A conta Microsoft com quem as planilhas dos grupos de liminares estão compartilhadas. O M80 só lê arquivos
          compartilhados com ela; nada é alterado no OneDrive.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-3 text-sm">
        {!connection?.configured && (
          <p className="text-destructive">
            O servidor está sem o app Microsoft configurado (MICROSOFT_CLIENT_ID/SECRET/REDIRECT_URI).
          </p>
        )}
        {connection?.configured && connected && (
          <>
            <Badge variant={connection.state === "ok" ? "secondary" : "destructive"}>
              {STATE_LABELS[connection.state ?? ""] ?? connection.state}
            </Badge>
            <span>
              {connection.account_name} {connection.account_email && `(${connection.account_email})`}
            </span>
            {connection.last_ok_at && (
              <span className="text-xs text-muted-foreground">última leitura {formatDateTime(connection.last_ok_at)}</span>
            )}
            {connection.detail && connection.state !== "ok" && (
              <span className="w-full text-xs text-muted-foreground">{connection.detail}</span>
            )}
          </>
        )}
        {connection?.configured && !connected && <span className="text-muted-foreground">Nenhuma conta conectada.</span>}
        {connection?.configured && (
          <div className="ml-auto flex gap-2">
            {connected && (
              <Button type="button" variant="outline" size="sm" disabled={isPending} onClick={disconnect}>
                Desconectar
              </Button>
            )}
            <Button type="button" size="sm" disabled={isPending} onClick={connect}>
              {connected ? "Reconectar" : "Conectar conta Microsoft"}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
