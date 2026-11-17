"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import type { z } from "zod"
import { Copy, KeyRound, Plus } from "lucide-react"
import { toast } from "sonner"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { createMCPTokenAction, revokeMCPTokenAction } from "@/actions/mcp"
import { formatDateTime } from "@/lib/format"
import { createMCPTokenSchema } from "@/schemas/mcp"
import type { MCPToken } from "@/schemas/mcp"

type FormData = z.infer<typeof createMCPTokenSchema.request>

export function MCPTokensCard({ tokens }: { tokens: MCPToken[] }) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  // The plain token exists only in the create response, so it lives here until the dialog
  // closes and is never fetched again.
  const [createdToken, setCreatedToken] = useState<string | null>(null)

  const form = useForm<FormData>({
    resolver: zodResolver(createMCPTokenSchema.request),
    defaultValues: { name: "" },
  })

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await createMCPTokenAction(data)
      if ("error" in result) {
        form.setError("root", { message: result.error })
        return
      }
      form.reset()
      setCreatedToken(result.data.token)
      router.refresh()
    })
  }

  function revoke(token: MCPToken) {
    startTransition(async () => {
      const result = await revokeMCPTokenAction(token.id)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success(`Token "${token.name}" revogado.`)
      router.refresh()
    })
  }

  async function copyToken() {
    if (!createdToken) return
    try {
      await navigator.clipboard.writeText(createdToken)
      toast.success("Token copiado.")
    } catch {
      toast.error("Não foi possível copiar. Selecione e copie à mão.")
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tokens</CardTitle>
        <CardDescription>
          Cada token age com o perfil e as agências de quem o criou. Só vale no endpoint MCP, nunca
          como sessão do painel.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-wrap items-end gap-3">
          <Field className="min-w-64 flex-1">
            <Controller
              name="name"
              control={form.control}
              render={({ field, fieldState }) => (
                <>
                  <FieldLabel htmlFor="mcp-token-name">Nome do novo token</FieldLabel>
                  <InputGroup className="bg-slate-200">
                    <InputGroupInput
                      {...field}
                      id="mcp-token-name"
                      placeholder="Claude Code - notebook"
                      aria-invalid={fieldState.invalid}
                      disabled={isPending}
                    />
                    <InputGroupAddon align="inline-end">
                      <KeyRound className="h-4 w-4" />
                    </InputGroupAddon>
                  </InputGroup>
                  {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                </>
              )}
            />
          </Field>
          <Button type="submit" disabled={isPending}>
            <Plus className="h-4 w-4" />
            Criar token
          </Button>
          {form.formState.errors.root && (
            <p className="w-full text-sm text-destructive">{form.formState.errors.root.message}</p>
          )}
        </form>

        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Nome</TableHead>
                <TableHead>Dono</TableHead>
                <TableHead>Último uso</TableHead>
                <TableHead>Criado em</TableHead>
                <TableHead className="w-28 text-right">Ações</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="py-6 text-center text-sm text-muted-foreground">
                    Nenhum token criado.
                  </TableCell>
                </TableRow>
              )}
              {tokens.map((token) => (
                <TableRow key={token.id} className="hover:bg-transparent">
                  <TableCell className="font-medium">
                    {token.name}
                    {token.revoked_at && (
                      <Badge variant="secondary" className="ml-2">
                        Revogado
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>{token.user_name ?? "—"}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {token.last_used_at ? formatDateTime(token.last_used_at) : "Nunca"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">{formatDateTime(token.created_at)}</TableCell>
                  <TableCell className="text-right">
                    {!token.revoked_at && (
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button type="button" variant="outline" size="sm" disabled={isPending}>
                            Revogar
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>Revogar &quot;{token.name}&quot;?</AlertDialogTitle>
                            <AlertDialogDescription>
                              Quem usa este token (Claude Code, rotinas) deixa de acessar o M80 na
                              hora. Não dá para desfazer: crie outro token se precisar.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Voltar</AlertDialogCancel>
                            <AlertDialogAction onClick={() => revoke(token)}>Revogar</AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>

      <Dialog open={createdToken !== null} onOpenChange={(open) => !open && setCreatedToken(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Copie o token agora</DialogTitle>
            <DialogDescription>
              Ele não será mostrado de novo. Guarde-o só na configuração do cliente MCP.
            </DialogDescription>
          </DialogHeader>
          <code className="rounded-md bg-slate-100 p-3 text-xs break-all">{createdToken}</code>
          <DialogFooter>
            <Button type="button" onClick={copyToken}>
              <Copy className="h-4 w-4" />
              Copiar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
