"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Ban, PowerOff, RotateCcw } from "lucide-react"
import { toast } from "sonner"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
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
import { Switch } from "@/components/ui/switch"
import {
  deactivateListenedChatAction,
  reactivateListenedChatAction,
  setListenedChatPDFModeAction,
  setListenedChatPromoModeAction,
  stopListenedChatAction,
} from "@/actions/listened-chats"
import { cn } from "@/lib/utils"
import type { ListenedChat } from "@/schemas/listened-chats"

const COLUMN_COUNT = 4

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString("pt-BR")
}

export function MonitoredChatsTable({ chats }: { chats: ListenedChat[] }) {
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

  function runAction(
    chat: ListenedChat,
    action: (id: string) => ReturnType<typeof stopListenedChatAction>,
    errorMessage: string,
  ) {
    markSaving(chat.id, true)

    startTransition(async () => {
      const result = await action(chat.id)
      markSaving(chat.id, false)

      if ("error" in result) {
        toast.error(result.error || errorMessage)
        return
      }

      router.refresh()
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Chats monitorados</CardTitle>
        <CardDescription>Chats aprovados que o bot está acompanhando agora.</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Chat</TableHead>
                <TableHead>Aprovado por</TableHead>
                <TableHead className="w-28 text-center">Modo PDF</TableHead>
                <TableHead className="w-32 text-center">Promoções</TableHead>
                <TableHead className="w-32 text-right">Ações</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {chats.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={COLUMN_COUNT}
                    className="py-8 text-center text-sm text-muted-foreground"
                  >
                    Nenhum chat sendo monitorado no momento.
                  </TableCell>
                </TableRow>
              )}
              {chats.map((chat) => {
                const isSaving = savingIds.has(chat.id)
                const isDeactivated = chat.status === "deactivated"

                return (
                  <TableRow
                    key={chat.id}
                    className={cn(
                      "hover:bg-transparent",
                      isDeactivated && "text-muted-foreground opacity-60",
                      isSaving && "opacity-60",
                    )}
                  >
                    <TableCell className="font-mono text-xs">
                      {chat.friendly_name ?? chat.chat_jid}
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col">
                        <span>{chat.approved_by_name || "—"}</span>
                        <span className="text-xs text-muted-foreground">
                          {formatDateTime(chat.updated_at)}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="text-center">
                      <PDFModeSwitch chat={chat} disabled={isSaving || isDeactivated} />
                    </TableCell>
                    <TableCell className="text-center">
                      <PromoModeSwitch chat={chat} disabled={isSaving || isDeactivated} />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end gap-1">
                        {isDeactivated ? (
                          <AlertDialog>
                            <AlertDialogTrigger asChild>
                              <Button
                                type="button"
                                variant="ghost"
                                size="icon"
                                title="Reativar"
                                disabled={isSaving}
                                className="h-8 w-8"
                              >
                                <RotateCcw className="h-4 w-4 text-emerald-600" />
                              </Button>
                            </AlertDialogTrigger>
                            <AlertDialogContent>
                              <AlertDialogHeader>
                                <AlertDialogTitle>Reativar chat</AlertDialogTitle>
                                <AlertDialogDescription>
                                  O bot volta a monitorar este chat normalmente.
                                </AlertDialogDescription>
                              </AlertDialogHeader>
                              <AlertDialogFooter>
                                <AlertDialogCancel>Voltar</AlertDialogCancel>
                                <AlertDialogAction
                                  onClick={() =>
                                    runAction(chat, reactivateListenedChatAction, "Erro ao reativar o chat.")
                                  }
                                >
                                  Reativar
                                </AlertDialogAction>
                              </AlertDialogFooter>
                            </AlertDialogContent>
                          </AlertDialog>
                        ) : (
                          <>
                            <AlertDialog>
                              <AlertDialogTrigger asChild>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="icon"
                                  title="Parar de monitorar"
                                  disabled={isSaving}
                                  className="h-8 w-8"
                                >
                                  <Ban className="h-4 w-4 text-destructive" />
                                </Button>
                              </AlertDialogTrigger>
                              <AlertDialogContent>
                                <AlertDialogHeader>
                                  <AlertDialogTitle>Parar de monitorar</AlertDialogTitle>
                                  <AlertDialogDescription>
                                    O bot vai parar de monitorar este chat. Para voltar a
                                    monitorá-lo depois, será preciso reativar diretamente no banco.
                                  </AlertDialogDescription>
                                </AlertDialogHeader>
                                <AlertDialogFooter>
                                  <AlertDialogCancel>Voltar</AlertDialogCancel>
                                  <AlertDialogAction
                                    onClick={() =>
                                      runAction(chat, stopListenedChatAction, "Erro ao parar de monitorar o chat.")
                                    }
                                  >
                                    Parar de monitorar
                                  </AlertDialogAction>
                                </AlertDialogFooter>
                              </AlertDialogContent>
                            </AlertDialog>

                            <AlertDialog>
                              <AlertDialogTrigger asChild>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="icon"
                                  title="Desativar"
                                  disabled={isSaving}
                                  className="h-8 w-8"
                                >
                                  <PowerOff className="h-4 w-4 text-destructive" />
                                </Button>
                              </AlertDialogTrigger>
                              <AlertDialogContent>
                                <AlertDialogHeader>
                                  <AlertDialogTitle>Desativar chat</AlertDialogTitle>
                                  <AlertDialogDescription>
                                    Diferente de “parar de monitorar”, isso é permanente: o chat
                                    continua aparecendo aqui (cinza), mas o bot nunca reabre
                                    sozinho se alguém mandar !registrar-chat de novo - só volta
                                    via “Reativar”.
                                  </AlertDialogDescription>
                                </AlertDialogHeader>
                                <AlertDialogFooter>
                                  <AlertDialogCancel>Voltar</AlertDialogCancel>
                                  <AlertDialogAction
                                    onClick={() =>
                                      runAction(chat, deactivateListenedChatAction, "Erro ao desativar o chat.")
                                    }
                                  >
                                    Desativar
                                  </AlertDialogAction>
                                </AlertDialogFooter>
                              </AlertDialogContent>
                            </AlertDialog>
                          </>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  )
}

// With this on, the bot posts the ticket PDF into the chat after every EMISSAO it
// managed to complete from Traveos. Off for a deactivated chat, where there is no
// emission to attach anything to.
//
// It writes the same column as the bot's !pdf command, so the switch is optimistic
// only until the server answers: someone may have flipped it from WhatsApp a second
// ago, and router.refresh() is what brings that back.
function PDFModeSwitch({ chat, disabled }: { chat: ListenedChat; disabled: boolean }) {
  const router = useRouter()
  const [enabled, setEnabled] = useState(chat.pdf_mode)
  const [isSaving, setIsSaving] = useState(false)

  async function handleChange(next: boolean) {
    setEnabled(next)
    setIsSaving(true)

    const result = await setListenedChatPDFModeAction(chat.id, next)
    setIsSaving(false)

    if ("error" in result) {
      setEnabled(!next)
      toast.error(result.error)
      return
    }

    setEnabled(result.chat.pdf_mode)
    toast.success(result.chat.pdf_mode ? "Modo PDF ligado." : "Modo PDF desligado.")
    router.refresh()
  }

  return (
    <Switch
      checked={enabled}
      disabled={disabled || isSaving}
      aria-label="Modo PDF"
      onCheckedChange={(next) => void handleChange(next)}
    />
  )
}

// With this on, a bare promotion link dropped in the chat makes the bot open the page
// and enrol the agency's CPFs. Off, a link stays a link - which is what it has to be
// in an issuance group.
//
// Same optimistic-until-answered shape as the PDF switch, so the row never lies about
// a state the API has not confirmed.
function PromoModeSwitch({ chat, disabled }: { chat: ListenedChat; disabled: boolean }) {
  const router = useRouter()
  const [enabled, setEnabled] = useState(chat.promo_mode)
  const [isSaving, setIsSaving] = useState(false)

  async function handleChange(next: boolean) {
    setEnabled(next)
    setIsSaving(true)

    const result = await setListenedChatPromoModeAction(chat.id, next)
    setIsSaving(false)

    if ("error" in result) {
      setEnabled(!next)
      toast.error(result.error)
      return
    }

    setEnabled(result.chat.promo_mode)
    toast.success(
      result.chat.promo_mode
        ? "Cadastro em promoções ligado."
        : "Cadastro em promoções desligado.",
    )
    router.refresh()
  }

  return (
    <Switch
      checked={enabled}
      disabled={disabled || isSaving}
      aria-label="Cadastro em promoções"
      onCheckedChange={(next) => void handleChange(next)}
    />
  )
}
