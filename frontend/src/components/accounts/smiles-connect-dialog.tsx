"use client"

import { useState, useTransition } from "react"
import Link from "next/link"
import { toast } from "sonner"
import { Loader2, Mail, Smartphone } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { InputOTP, InputOTPGroup, InputOTPSlot } from "@/components/ui/input-otp"
import { Label } from "@/components/ui/label"
import {
  browserChallengeSmilesLoginAction,
  completeBrowserChallengeSmilesLoginAction,
  confirmSmilesLoginAction,
  type ConnectArea,
} from "@/actions/statement"
import { challengeBridgeReady, requestSmilesChallenge } from "@/lib/smiles-challenge-extension"
import type { SmilesAuthenticator } from "@/schemas/inventory"

// The code Smiles sends is six digits on every channel measured.
const CODE_LENGTH = 6

interface Props {
  accountId: string
  accountLabel: string
  authenticators: SmilesAuthenticator[]
  open: boolean
  onOpenChange: (open: boolean) => void
  // Called once the account is connected, so the caller can refresh what it shows.
  onConnected: () => void
  // Called when the login window lapsed and the operator has to start over, so the caller can
  // drop the channels it is holding.
  onExpired: () => void
  // The staff screens or the statement client's portal: same conversation, different routes,
  // and a different extension to point at when it is missing.
  area?: ConnectArea
}

// The conversation Smiles requires, as two steps in one dialog: pick where the code goes, then
// type it. It is deliberately not a page - the operator is in the middle of something else, and
// the whole exchange lasts a minute.
export function SmilesConnectDialog({
  accountId,
  accountLabel,
  authenticators,
  open,
  onOpenChange,
  onConnected,
  onExpired,
  area = "staff",
}: Props) {
  const [sentTo, setSentTo] = useState<string | null>(null)
  const [code, setCode] = useState("")
  // Shown inside the dialog rather than as a toast: a wrong code is answered by retyping it
  // right here, and a toast would take the message away from where the answer goes.
  const [error, setError] = useState<string | null>(null)
  // The portal answers a missing extension with the install guide, not just a sentence.
  const [missingExtension, setMissingExtension] = useState(false)
  const [isPending, startTransition] = useTransition()

  // The tenant's challenge runs in the operator's own browser - the only client the edge treats
  // as a person. Without the extension there is no path: every automated client gets 406.
  async function askSmilesForTheCode(
    authenticator: SmilesAuthenticator,
  ): Promise<{ error: string } | { sentTo: string }> {
    if (!challengeBridgeReady()) {
      setMissingExtension(true)
      return {
        error:
          area === "portal"
            ? "Para receber o código, instale a extensão M80 Extrato neste navegador e recarregue a página."
            : "Instale a extensão M80 neste navegador para receber o código de conexão.",
      }
    }
    setMissingExtension(false)
    const material = await browserChallengeSmilesLoginAction(accountId, authenticator.id, area)
    if ("error" in material) return { error: material.error }
    const answer = await requestSmilesChallenge({
      clientId: material.clientId,
      mfaToken: material.mfaToken,
      authenticatorId: material.authenticatorId,
    })
    if ("error" in answer) return { error: answer.error }
    const completed = await completeBrowserChallengeSmilesLoginAction(
      accountId,
      authenticator.id,
      answer.oobCode,
      area,
    )
    if ("error" in completed) return { error: completed.error }
    return { sentTo: completed.sentTo }
  }

  function requestCode(authenticator: SmilesAuthenticator) {
    setError(null)
    startTransition(async () => {
      const result = await askSmilesForTheCode(authenticator)
      if ("error" in result) {
        // A lapsed login cannot be recovered from inside the dialog: the operator starts again.
        if (result.error.includes("expirou")) {
          toast.error(result.error)
          onExpired()
          onOpenChange(false)
          return
        }
        setError(result.error)
        return
      }
      setSentTo(result.sentTo)
    })
  }

  function submitCode() {
    if (code.length < CODE_LENGTH) {
      setError("Digite os seis dígitos do código.")
      return
    }
    setError(null)
    startTransition(async () => {
      const result = await confirmSmilesLoginAction(accountId, code, area)
      if ("error" in result) {
        if (result.error.includes("expirou")) {
          toast.error(result.error)
          onExpired()
          onOpenChange(false)
          return
        }
        // The code stays on screen so the operator can see what they typed and fix it. Nothing
        // asks Smiles for another code: it counts failed attempts and throttles the channel.
        setError(result.error)
        return
      }
      toast.success("Conta conectada. O extrato aparece assim que a primeira leitura terminar.")
      reset()
      onOpenChange(false)
      onConnected()
    })
  }

  function reset() {
    setSentTo(null)
    setCode("")
    setError(null)
    setMissingExtension(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset()
        onOpenChange(next)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Conectar {accountLabel} na Smiles</DialogTitle>
          <DialogDescription>
            {sentTo
              ? `A Smiles enviou um código para ${sentTo}. Digite os seis dígitos abaixo.`
              : "A Smiles exige um código de verificação. Escolha onde quer recebê-lo."}
          </DialogDescription>
        </DialogHeader>

        {sentTo ? (
          <div className="flex flex-col items-center gap-3 py-2">
            <Label htmlFor="smiles-code" className="sr-only">
              Código de verificação
            </Label>
            <InputOTP
              id="smiles-code"
              maxLength={CODE_LENGTH}
              value={code}
              onChange={setCode}
              disabled={isPending}
              autoFocus
            >
              <InputOTPGroup>
                {Array.from({ length: CODE_LENGTH }, (_, index) => (
                  <InputOTPSlot key={index} index={index} />
                ))}
              </InputOTPGroup>
            </InputOTP>
            <p className="text-muted-foreground text-center text-xs">
              O código vale por poucos minutos. A Smiles limita as tentativas, então confira os
              dígitos antes de enviar.
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-2 py-2">
            {authenticators.map((authenticator) => (
              <Button
                key={authenticator.id}
                variant="outline"
                className="justify-start"
                disabled={isPending}
                onClick={() => requestCode(authenticator)}
              >
                {authenticator.channel === "sms" ? (
                  <Smartphone className="size-4" />
                ) : (
                  <Mail className="size-4" />
                )}
                {authenticator.channel === "sms" ? "SMS" : "E-mail"} — {authenticator.target}
              </Button>
            ))}
            {authenticators.length === 0 && (
              <p className="text-muted-foreground text-sm">
                A Smiles não devolveu nenhum canal para esta conta. Confira os dados de contato
                cadastrados no site da Smiles.
              </p>
            )}
          </div>
        )}

        {error && <p className="text-destructive text-sm">{error}</p>}
        {missingExtension && area === "portal" && (
          <Link href="/portal/extensao" className="text-sm font-medium text-sky-700 underline">
            Como instalar a extensão
          </Link>
        )}

        <DialogFooter>
          {sentTo ? (
            <Button onClick={submitCode} disabled={isPending || code.length < CODE_LENGTH}>
              {isPending && <Loader2 className="size-4 animate-spin" />}
              Confirmar código
            </Button>
          ) : (
            <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={isPending}>
              Cancelar
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
