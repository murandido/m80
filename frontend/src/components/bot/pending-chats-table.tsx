"use client"

import { useCallback, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Check, X } from "lucide-react"
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
import {
  approveListenedChatAction,
  denyListenedChatAction,
} from "@/actions/listened-chats"
import { useSSE } from "@/hooks/use-sse"
import { cn } from "@/lib/utils"
import type { ListenedChat } from "@/schemas/listened-chats"

const COLUMN_COUNT = 4

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString("pt-BR")
}

export function PendingChatsTable({ chats }: { chats: ListenedChat[] }) {
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

  function decide(
    chat: ListenedChat,
    action: typeof approveListenedChatAction,
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

      // A decided chat leaves the pending list, so a plain refetch (rather
      // than an optimistic patch) is enough - the row just disappears once
      // the Server Component re-fetches with the updated status.
      // TODO: whether to notify the WhatsApp chat itself on approve/deny is
      // an open product decision, not implemented here (out of scope).
      router.refresh()
    })
  }

  // The bot creates/reopens a pending request from a completely separate
  // process, so this list can't just wait for a local mutation like
  // approve/deny do above - it needs to be told from the outside. Same
  // bridge as the bot connection status: refetch on the event, never trust
  // any payload (there isn't one).
  const refresh = useCallback(() => router.refresh(), [router])
  useSSE(
    "/api/events",
    { "listened_chat.requested": refresh },
    { onReconnect: refresh },
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>Chats pendentes</CardTitle>
        <CardDescription>
          Solicitações de !registrar-chat aguardando aprovação.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Chat</TableHead>
                <TableHead>Solicitado por</TableHead>
                <TableHead>Solicitado em</TableHead>
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
                    Nenhum chat pendente no momento.
                  </TableCell>
                </TableRow>
              )}
              {chats.map((chat) => {
                const isSaving = savingIds.has(chat.id)

                return (
                  <TableRow key={chat.id} className={cn("hover:bg-transparent", isSaving && "opacity-60")}>
                    <TableCell className="font-mono text-xs">
                      {chat.friendly_name ?? chat.chat_jid}
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col">
                        <span>{chat.requested_by_name}</span>
                        <span className="text-xs text-muted-foreground">
                          {chat.requested_by_phone}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {formatDateTime(chat.created_at)}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end gap-1">
                        <AlertDialog>
                          <AlertDialogTrigger asChild>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              title="Aprovar"
                              disabled={isSaving}
                              className="h-8 w-8"
                            >
                              <Check className="h-4 w-4 text-emerald-600" />
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>Aprovar chat</AlertDialogTitle>
                              <AlertDialogDescription>
                                O bot passará a monitorar este chat.
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>Voltar</AlertDialogCancel>
                              <AlertDialogAction
                                onClick={() =>
                                  decide(chat, approveListenedChatAction, "Erro ao aprovar chat.")
                                }
                              >
                                Aprovar
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
                              title="Negar"
                              disabled={isSaving}
                              className="h-8 w-8"
                            >
                              <X className="h-4 w-4 text-destructive" />
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>Negar chat</AlertDialogTitle>
                              <AlertDialogDescription>
                                O bot não vai monitorar este chat.
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>Voltar</AlertDialogCancel>
                              <AlertDialogAction
                                onClick={() =>
                                  decide(chat, denyListenedChatAction, "Erro ao negar chat.")
                                }
                              >
                                Negar
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
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
