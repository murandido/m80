"use client"

import { useCallback, useState, useTransition } from "react"
import { QRCodeSVG } from "qrcode.react"
import { Bot as BotIcon, LogOut, QrCode, Smartphone, XCircle } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useSSE } from "@/hooks/use-sse"
import {
  cancelPairingAction,
  getBotStatusAction,
  logoutBotAction,
  startPairingAction,
} from "@/actions/bot"
import type { BotStatus } from "@/schemas/bot"

const STATE_LABEL: Record<BotStatus["state"], string> = {
  disconnected: "Desconectado",
  pairing: "Pareando",
  connected: "Conectado",
}

const STATE_VARIANT: Record<
  BotStatus["state"],
  "outline" | "secondary" | "default"
> = {
  disconnected: "outline",
  pairing: "secondary",
  connected: "default",
}

export function BotStatusCard({
  initialStatus,
}: {
  initialStatus: BotStatus | null
}) {
  const [status, setStatus] = useState(initialStatus)
  const [error, setError] = useState<string | null>(null)
  const [phone, setPhone] = useState("")
  const [isPending, startTransition] = useTransition()

  // The SSE event carries no payload by convention - refetch the
  // authoritative state instead of trusting anything on the wire. This also
  // picks up each QR code refresh during pairing, since every refresh goes
  // through the same "bot.status" broadcast.
  const refresh = useCallback(() => {
    startTransition(async () => {
      const result = await getBotStatusAction()
      if ("status" in result) {
        setStatus(result.status)
      }
    })
  }, [])

  useSSE("/api/events", { "bot.status": refresh })

  function handleStartPairing(method: "phone" | "qr") {
    if (method === "phone" && phone.trim() === "") {
      setError("Informe o telefone (DDD + número) para parear.")
      return
    }

    setError(null)
    startTransition(async () => {
      const result = await startPairingAction(method, method === "phone" ? phone : undefined)
      if ("error" in result) {
        setError(result.error)
        return
      }
      setStatus(result.status)
    })
  }

  function handleCancel() {
    setError(null)
    startTransition(async () => {
      const result = await cancelPairingAction()
      if ("error" in result) {
        setError(result.error)
        return
      }
      setStatus(result.status)
    })
  }

  function handleLogout() {
    setError(null)
    startTransition(async () => {
      const result = await logoutBotAction()
      if ("error" in result) {
        setError(result.error)
        return
      }
      setStatus(result.status)
    })
  }

  const state = status?.state ?? "disconnected"

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BotIcon className="h-5 w-5" />
          Bot do WhatsApp
        </CardTitle>
        <CardDescription>
          Status da sessão conectada ao número do painel.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center gap-2">
          <Badge variant={STATE_VARIANT[state]}>{STATE_LABEL[state]}</Badge>
          {status?.jid && (
            <span className="text-sm text-slate-500">{status.jid}</span>
          )}
        </div>

        {error && <p className="text-destructive text-sm">{error}</p>}

        {state === "pairing" &&
          status?.pair_method === "phone" &&
          status.pair_code && (
            <div className="flex flex-col gap-1">
              <span className="text-sm text-slate-500">
                Digite este código em Aparelhos conectados → Conectar com
                número de telefone:
              </span>
              <span className="font-mono text-2xl tracking-widest">
                {status.pair_code}
              </span>
            </div>
          )}

        {state === "pairing" &&
          status?.pair_method === "qr" &&
          status.pair_code && (
            <div className="flex flex-col items-center gap-2">
              <span className="text-sm text-slate-500">
                Escaneie com o WhatsApp → Aparelhos conectados:
              </span>
              <QRCodeSVG value={status.pair_code} size={200} />
            </div>
          )}

        {state === "pairing" && status?.pair_method === "qr" && !status.pair_code && (
          <p className="text-sm text-slate-500">Gerando QR code...</p>
        )}

        {state === "disconnected" && (
          <div className="flex flex-col gap-1">
            <span className="text-sm text-slate-500">
              Telefone do WhatsApp a parear (DDD + número):
            </span>
            <Input
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              placeholder="11999999999"
              disabled={isPending}
              className="max-w-[220px]"
            />
          </div>
        )}

        <div className="flex flex-wrap gap-2">
          {state === "disconnected" && (
            <>
              <Button onClick={() => handleStartPairing("phone")} disabled={isPending}>
                {isPending ? <Spinner /> : <Smartphone className="h-4 w-4" />}
                Iniciar pareamento
              </Button>
              <Button
                variant="outline"
                onClick={() => handleStartPairing("qr")}
                disabled={isPending}
              >
                <QrCode className="h-4 w-4" />
                Usar QR code
              </Button>
            </>
          )}

          {state === "pairing" && (
            <Button variant="outline" onClick={handleCancel} disabled={isPending}>
              {isPending ? <Spinner /> : <XCircle className="h-4 w-4" />}
              Cancelar
            </Button>
          )}

          {state === "connected" && (
            <Button variant="destructive" onClick={handleLogout} disabled={isPending}>
              {isPending ? <Spinner /> : <LogOut className="h-4 w-4" />}
              Logout
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
