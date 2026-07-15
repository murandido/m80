"use client"

import { useEffect, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Hash, PhoneCall, Save, User } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { Client, updateClientSchema } from "@/schemas/client"
import { updateClientAction } from "@/actions/client"

type FormData = z.infer<typeof updateClientSchema.request>

interface EditClientPanelProps {
  client: Client
  isOpen: boolean
  onSaved: () => void
}

export function EditClientPanel({ client, isOpen, onSaved }: EditClientPanelProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  const defaultValues: FormData = {
    name: client.name,
    phone: client.phone ?? "",
  }

  const form = useForm<FormData>({
    resolver: zodResolver(updateClientSchema.request),
    mode: "onTouched",
    defaultValues,
  })

  // Reset form whenever the panel closes
  useEffect(() => {
    if (!isOpen) {
      form.reset(defaultValues)
    }
  }, [isOpen]) // eslint-disable-line react-hooks/exhaustive-deps

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updateClientAction(client.id, data)
      if (result.error) {
        form.setError("root", { message: result.error })
        return
      }
      onSaved()
      router.refresh()
    })
  }

  return (
    <div
      className={cn(
        "grid transition-all duration-300 ease-in-out",
        isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
    >
      <div className="overflow-hidden">
        <div className="rounded-b-md border border-t-0 border-slate-200 p-4">
          <div className="mb-4">
            <h2 className="text-sm font-semibold text-slate-800">
              Informações do cliente
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Veja e/ou atualize as informações do cliente aqui!
            </p>
          </div>

          <form onSubmit={form.handleSubmit(onSubmit)}>
            <Field className="flex flex-col gap-5">
              {/* UUID — read-only */}
              <div className="flex flex-col gap-1 text-sm">
                <FieldLabel htmlFor="edit-uuid">UUID</FieldLabel>
                <InputGroup className="bg-slate-100">
                  <InputGroupInput
                    id="edit-uuid"
                    type="text"
                    value={client.id}
                    readOnly
                    className="cursor-default select-all text-slate-400"
                  />
                  <InputGroupAddon align="inline-end">
                    <Hash className="h-4 w-4 text-slate-400" />
                  </InputGroupAddon>
                </InputGroup>
              </div>

              {/* name */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="name"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-name">
                        Nome <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-name"
                          type="text"
                          aria-invalid={fieldState.invalid}
                          placeholder="João Silva"
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <User className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              {/* phone */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="phone"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-phone">Telefone</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-phone"
                          type="tel"
                          aria-invalid={fieldState.invalid}
                          placeholder="11999999999"
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <PhoneCall className="text-foreground h-4 w-4" />
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              <Separator />

              {form.formState.errors.root && (
                <p className="text-destructive text-sm">
                  {form.formState.errors.root.message}
                </p>
              )}

              <Button
                type="submit"
                className="h-full bg-slate-800 hover:bg-slate-900"
                disabled={isPending}
              >
                <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
                  {isPending ? <Spinner /> : <Save className="h-4 w-4" />}
                  {isPending ? "Salvando..." : "Salvar Alterações"}
                </div>
              </Button>
            </Field>
          </form>
        </div>
      </div>
    </div>
  )
}
