"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Copy, KeyRound, Plus, TriangleAlert } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"
import { createMcpTokenAction } from "@/actions/mcp-token"
import { copyToClipboard } from "@/lib/clipboard"
import { createMcpTokenSchema } from "@/schemas/mcp-token"

type FormData = z.infer<typeof createMcpTokenSchema.request>

interface CreatedToken {
  token: string
  command: string
}

export function CreateMcpTokenDialog() {
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [isPending, startTransition] = useTransition()
  // Held only in this dialog's state: the API keeps just a hash, so once the dialog closes
  // the plaintext is gone for good.
  const [created, setCreated] = useState<CreatedToken | null>(null)

  const form = useForm<FormData>({
    resolver: zodResolver(createMcpTokenSchema.request),
    mode: "onTouched",
    defaultValues: { name: "" },
  })

  function handleOpenChange(next: boolean) {
    if (!next) {
      form.reset()
      setCreated(null)
    }
    setOpen(next)
  }

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await createMcpTokenAction(data)

      if ("error" in result) {
        form.setError("root", { message: result.error })
        return
      }

      // The panel's own origin, not a configured one: the relay at /api/mcp is served by
      // whichever panel the developer is logged into.
      const endpoint = `${window.location.origin}/api/mcp`
      setCreated({
        token: result.token,
        command: `claude mcp add --scope user --transport http m80 ${endpoint} --header "Authorization: Bearer ${result.token}"`,
      })
      router.refresh()
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button type="button" size="sm">
          <Plus className="h-4 w-4" />
          Novo token
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>Token criado</DialogTitle>
              <DialogDescription>
                Copie o token agora e guarde em lugar seguro.
              </DialogDescription>
            </DialogHeader>

            <Alert variant="destructive">
              <TriangleAlert className="h-4 w-4" />
              <AlertTitle>Ele não será mostrado de novo</AlertTitle>
              <AlertDescription>
                O M80 não guarda o token em si. Se perdê-lo, revogue este e crie outro.
              </AlertDescription>
            </Alert>

            <div className="flex flex-col gap-1 text-sm">
              <FieldLabel htmlFor="created-mcp-token">Token</FieldLabel>
              <InputGroup>
                <InputGroupInput
                  id="created-mcp-token"
                  readOnly
                  value={created.token}
                  className="font-mono text-xs"
                  onFocus={(event) => event.currentTarget.select()}
                />
                <InputGroupAddon align="inline-end">
                  <InputGroupButton
                    size="icon-xs"
                    title="Copiar token"
                    onClick={() => copyToClipboard(created.token, "Token copiado.")}
                  >
                    <Copy />
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
            </div>

            <div className="flex flex-col gap-1 text-sm">
              <span className="font-medium">Para usar no Claude Code, rode no terminal:</span>
              <div className="flex items-start gap-2 rounded-md border bg-slate-50 p-2">
                <code className="flex-1 font-mono text-xs break-all text-slate-700">
                  {created.command}
                </code>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  title="Copiar comando"
                  className="h-7 w-7 shrink-0"
                  onClick={() => copyToClipboard(created.command, "Comando copiado.")}
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
            </div>

            <DialogFooter>
              <Button type="button" onClick={() => handleOpenChange(false)}>
                Concluir
              </Button>
            </DialogFooter>
          </>
        ) : (
          <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Novo token</DialogTitle>
              <DialogDescription>
                Dê um nome que diga onde ele será usado, como o computador ou o projeto.
              </DialogDescription>
            </DialogHeader>

            <Field className="flex flex-col gap-1 text-sm">
              <Controller
                name="name"
                control={form.control}
                render={({ field, fieldState }) => (
                  <>
                    <FieldLabel htmlFor="mcp-token-name">
                      Nome <span className="text-destructive">*</span>
                    </FieldLabel>
                    <InputGroup>
                      <InputGroupInput
                        {...field}
                        id="mcp-token-name"
                        type="text"
                        maxLength={100}
                        aria-invalid={fieldState.invalid}
                        placeholder="Notebook do trabalho"
                        disabled={isPending}
                        autoFocus
                      />
                      <InputGroupAddon align="inline-end">
                        <KeyRound className="text-foreground h-4 w-4" />
                      </InputGroupAddon>
                    </InputGroup>
                    {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
                  </>
                )}
              />
            </Field>

            {form.formState.errors.root && (
              <p className="text-sm text-destructive">{form.formState.errors.root.message}</p>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={isPending}
                onClick={() => handleOpenChange(false)}
              >
                Cancelar
              </Button>
              <Button type="submit" disabled={isPending}>
                {isPending && <Spinner />}
                Criar token
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
