"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Ban } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
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
import { revokeMcpTokenAction } from "@/actions/mcp-token"
import { cn } from "@/lib/utils"
import type { McpToken } from "@/schemas/mcp-token"
import { CreateMcpTokenDialog } from "./create-mcp-token-dialog"

const COLUMN_COUNT = 5

function formatDateTime(iso: string | null): string {
  return iso ? new Date(iso).toLocaleString("pt-BR") : "Nunca"
}

export function McpTokensTable({ tokens }: { tokens: McpToken[] }) {
  const router = useRouter()
  const [, startTransition] = useTransition()
  const [revokingId, setRevokingId] = useState<string | null>(null)

  function revoke(mcpToken: McpToken) {
    setRevokingId(mcpToken.id)

    startTransition(async () => {
      const result = await revokeMcpTokenAction(mcpToken.id)
      setRevokingId(null)

      if ("error" in result) {
        toast.error(result.error)
        return
      }

      toast.success(`Token "${mcpToken.name}" revogado.`)
      router.refresh()
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Seus tokens</CardTitle>
        <CardDescription>
          Cada token deixa o Claude Code consultar o M80 em seu nome. Revogue o que não usar mais.
        </CardDescription>
        <CardAction>
          <CreateMcpTokenDialog />
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Nome</TableHead>
                <TableHead>Criado em</TableHead>
                <TableHead>Último uso</TableHead>
                <TableHead>Situação</TableHead>
                <TableHead className="w-32 text-right">Ações</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={COLUMN_COUNT}
                    className="py-8 text-center text-sm text-muted-foreground"
                  >
                    Nenhum token criado.
                  </TableCell>
                </TableRow>
              )}
              {tokens.map((mcpToken) => {
                const isRevoked = mcpToken.revoked_at !== null
                const isRevoking = revokingId === mcpToken.id

                return (
                  <TableRow
                    key={mcpToken.id}
                    className={cn("hover:bg-transparent", (isRevoked || isRevoking) && "opacity-60")}
                  >
                    <TableCell className="font-medium">{mcpToken.name}</TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {formatDateTime(mcpToken.created_at)}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {formatDateTime(mcpToken.last_used_at)}
                    </TableCell>
                    <TableCell>
                      {isRevoked ? (
                        <Badge variant="secondary">Revogado</Badge>
                      ) : (
                        <Badge className="bg-emerald-600 text-white">Ativo</Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end">
                        {!isRevoked && (
                          <AlertDialog>
                            <AlertDialogTrigger asChild>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                disabled={isRevoking}
                                className="text-destructive hover:text-destructive"
                              >
                                <Ban className="h-4 w-4" />
                                Revogar
                              </Button>
                            </AlertDialogTrigger>
                            <AlertDialogContent>
                              <AlertDialogHeader>
                                <AlertDialogTitle>Revogar token</AlertDialogTitle>
                                <AlertDialogDescription>
                                  O token &quot;{mcpToken.name}&quot; deixa de funcionar na hora, e
                                  quem o estiver usando precisará de um novo. Isso não pode ser
                                  desfeito.
                                </AlertDialogDescription>
                              </AlertDialogHeader>
                              <AlertDialogFooter>
                                <AlertDialogCancel>Voltar</AlertDialogCancel>
                                <AlertDialogAction
                                  className="bg-destructive text-white hover:bg-destructive/90"
                                  onClick={() => revoke(mcpToken)}
                                >
                                  Revogar
                                </AlertDialogAction>
                              </AlertDialogFooter>
                            </AlertDialogContent>
                          </AlertDialog>
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
