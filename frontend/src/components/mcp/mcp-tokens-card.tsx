"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm, useWatch } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import type { z } from "zod"
import { Copy, KeyRound, Plus } from "lucide-react"
import { toast } from "sonner"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
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
import { createMCPTokenSchema, mcpFreeWriteTools } from "@/schemas/mcp"
import type { MCPToken } from "@/schemas/mcp"
import type { User } from "@/schemas/user"

type FormInput = z.input<typeof createMCPTokenSchema.request>
type FormData = z.output<typeof createMCPTokenSchema.request>

// The sentinel the owner select uses for "me": an empty string is not a valid Select value.
const SELF = "self"

function policyLabel(token: MCPToken): string {
  if (token.allowed_write_tools == null && !token.limits) return "Escreve tudo direto"
  const parts: string[] = []
  const tools = token.allowed_write_tools
  if (tools == null) parts.push("todas as escritas liberadas")
  else if (tools.length === 0) parts.push("tudo vira proposta")
  else parts.push(`sozinho: ${tools.join(", ")}`)
  if (token.limits?.max_batch_points) parts.push(`até ${token.limits.max_batch_points.toLocaleString("pt-BR")} pts/lote`)
  if (token.limits?.max_batches_per_day) parts.push(`${token.limits.max_batches_per_day} lotes/dia`)
  return parts.join(" · ")
}

export function MCPTokensCard({ tokens, owners }: { tokens: MCPToken[]; owners: User[] }) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  // The plain token exists only in the create response, so it lives here until the dialog
  // closes and is never fetched again.
  const [createdToken, setCreatedToken] = useState<string | null>(null)

  const form = useForm<FormInput, unknown, FormData>({
    resolver: zodResolver(createMCPTokenSchema.request),
    defaultValues: {
      name: "",
      user_id: SELF,
      restricted: false,
      allowed_write_tools: [],
      max_batch_points: "",
      max_batches_per_day: "",
    },
  })
  const restricted = useWatch({ control: form.control, name: "restricted" })

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await createMCPTokenAction({ ...data, user_id: data.user_id === SELF ? undefined : data.user_id })
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
          Cada token age com o perfil e as agências do dono. Só vale no endpoint MCP, nunca como sessão
          do painel. Para uma rotina, use um usuário próprio do agente e restrinja o que ele escreve
          sozinho: o resto vira proposta.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4 rounded-md border p-4">
          <div className="flex flex-wrap items-end gap-3">
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
                        placeholder="Rotina das liminares"
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
            <Field className="min-w-56">
              <Controller
                name="user_id"
                control={form.control}
                render={({ field }) => (
                  <>
                    <FieldLabel htmlFor="mcp-token-owner">Dono (age em nome de)</FieldLabel>
                    <Select value={field.value ?? SELF} onValueChange={field.onChange} disabled={isPending}>
                      <SelectTrigger id="mcp-token-owner" className="bg-slate-200">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={SELF}>Eu</SelectItem>
                        {owners.map((owner) => (
                          <SelectItem key={owner.id} value={owner.id}>
                            {owner.name} ({owner.role})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </>
                )}
              />
            </Field>
          </div>

          <Controller
            name="restricted"
            control={form.control}
            render={({ field }) => (
              <label className="flex items-center gap-2 text-sm">
                <Switch checked={field.value} onCheckedChange={field.onChange} disabled={isPending} />
                Restringir o que este token escreve sozinho (o resto vira proposta)
              </label>
            )}
          />

          {restricted && (
            <div className="flex flex-col gap-3 rounded-md bg-slate-50 p-3">
              <Controller
                name="allowed_write_tools"
                control={form.control}
                render={({ field }) => (
                  <div className="grid gap-2 sm:grid-cols-2">
                    {mcpFreeWriteTools.map((tool) => (
                      <label key={tool.value} className="flex items-center gap-2 text-sm">
                        <Switch
                          checked={field.value.includes(tool.value)}
                          onCheckedChange={(checked) =>
                            field.onChange(
                              checked
                                ? [...field.value, tool.value]
                                : field.value.filter((value) => value !== tool.value),
                            )
                          }
                          disabled={isPending}
                        />
                        {tool.label}
                      </label>
                    ))}
                  </div>
                )}
              />
              <p className="text-xs text-muted-foreground">
                Nenhuma marcada: tudo o que ele tentar escrever vira proposta.
              </p>
              <div className="flex flex-wrap gap-3">
                {(
                  [
                    ["max_batch_points", "Máx. pontos por lote"],
                    ["max_batches_per_day", "Máx. lotes por dia"],
                  ] as const
                ).map(([name, label]) => (
                  <Field key={name} className="w-48">
                    <Controller
                      name={name}
                      control={form.control}
                      render={({ field, fieldState }) => (
                        <>
                          <FieldLabel htmlFor={`mcp-${name}`}>{label}</FieldLabel>
                          <InputGroup className="bg-slate-200">
                            <InputGroupInput
                              id={`mcp-${name}`}
                              type="number"
                              min={1}
                              placeholder="sem limite"
                              value={field.value == null ? "" : String(field.value)}
                              onChange={(event) => field.onChange(event.target.value)}
                              onBlur={field.onBlur}
                              aria-invalid={fieldState.invalid}
                              disabled={isPending}
                            />
                          </InputGroup>
                          {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                        </>
                      )}
                    />
                  </Field>
                ))}
              </div>
            </div>
          )}

          <div className="flex items-center gap-3">
            <Button type="submit" disabled={isPending}>
              <Plus className="h-4 w-4" />
              Criar token
            </Button>
            {form.formState.errors.root && (
              <p className="text-sm text-destructive">{form.formState.errors.root.message}</p>
            )}
          </div>
        </form>

        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Nome</TableHead>
                <TableHead>Dono</TableHead>
                <TableHead>Política</TableHead>
                <TableHead>Último uso</TableHead>
                <TableHead>Criado em</TableHead>
                <TableHead className="w-28 text-right">Ações</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="py-6 text-center text-sm text-muted-foreground">
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
                  <TableCell className="max-w-xs text-xs text-muted-foreground">{policyLabel(token)}</TableCell>
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
