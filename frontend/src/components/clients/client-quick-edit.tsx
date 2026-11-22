"use client"

import { useState, useTransition } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { ArrowLeft, Loader2, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { FieldError, FieldLabel } from "@/components/ui/field"
import { ClientAgencyField } from "@/components/clients/client-agency-field"
import { deleteClientAction, updateClientAction } from "@/actions/client"
import { type Client, updateClientSchema } from "@/schemas/client"
import type { Agency } from "@/schemas/agency"

type FormData = z.infer<typeof updateClientSchema.request>

interface ClientQuickEditProps {
  client: Client
  agencies: Agency[]
  // Only a dev moves a client between agencies - and is the only one who sees the clients
  // that have none yet, which is exactly when the field is needed.
  showAgency: boolean
  onSaved: (client: Client) => void
  onDeleted: (clientId: string) => void
  onBack: () => void
}

// The client register, one client at a time, inside the "Vincular cliente..." list: the
// payments control is where a wrong phone or a misspelt name is noticed, so it is fixed
// there instead of on a screen of its own.
export function ClientQuickEdit({
  client,
  agencies,
  showAgency,
  onSaved,
  onDeleted,
  onBack,
}: ClientQuickEditProps) {
  const [isPending, startTransition] = useTransition()
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const form = useForm<FormData>({
    resolver: zodResolver(updateClientSchema.request),
    defaultValues: {
      name: client.name,
      phone: client.phone ?? "",
      // The API requires the agency on every update; whoever does not see the field sends
      // the one the client already has.
      agency_id: client.agency_id ?? "",
    },
  })

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updateClientAction(client.id, data)
      if ("error" in result && result.error) {
        form.setError("root", { message: result.error })
        return
      }
      onSaved({
        ...client,
        // Stored in capitals by the API; mirrored so the list does not flicker to the
        // typed spelling until the next refresh.
        name: data.name.trim().toUpperCase(),
        phone: data.phone?.trim() || null,
        agency_id: data.agency_id,
      })
    })
  }

  function handleDelete() {
    startTransition(async () => {
      const result = await deleteClientAction(client.id)
      if ("error" in result && result.error) {
        setConfirmingDelete(false)
        form.setError("root", { message: result.error })
        return
      }
      onDeleted(client.id)
    })
  }

  const nameError = form.formState.errors.name

  return (
    <form
      onSubmit={form.handleSubmit(onSubmit)}
      // Enter inside the form saves it; it must not reach the combobox around it.
      onKeyDown={(event) => event.stopPropagation()}
      className="flex flex-col gap-3"
    >
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onBack}
          disabled={isPending}
          className="h-7 px-2 text-slate-600"
        >
          <ArrowLeft className="h-4 w-4" />
          Voltar
        </Button>
        <span className="min-w-0 flex-1 truncate text-right text-sm font-medium text-slate-800">
          {client.name}
        </span>
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <FieldLabel htmlFor={`client-name-${client.id}`}>
          Nome <span className="text-destructive">*</span>
        </FieldLabel>
        <Input
          id={`client-name-${client.id}`}
          autoFocus
          {...form.register("name")}
          disabled={isPending}
          className="bg-slate-200 uppercase"
        />
        {nameError && <FieldError errors={[nameError]} />}
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <FieldLabel htmlFor={`client-phone-${client.id}`}>Telefone</FieldLabel>
        <Input
          id={`client-phone-${client.id}`}
          {...form.register("phone")}
          inputMode="tel"
          placeholder="Opcional"
          disabled={isPending}
          className="bg-slate-200"
        />
      </div>

      {showAgency && (
        <div className="flex flex-col gap-1 text-sm">
          <ClientAgencyField
            control={form.control}
            name="agency_id"
            agencies={agencies}
            disabled={isPending}
          />
        </div>
      )}

      {form.formState.errors.root && (
        <p className="text-destructive text-sm">
          {form.formState.errors.root.message}
        </p>
      )}

      {confirmingDelete ? (
        <div className="flex flex-col gap-2 rounded-md border border-red-200 bg-red-50 p-2">
          <p className="text-xs text-red-900">
            As vendas deste cliente ficam sem cliente. Excluir {client.name}?
          </p>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setConfirmingDelete(false)}
              disabled={isPending}
              className="flex-1"
            >
              Cancelar
            </Button>
            <Button
              type="button"
              size="sm"
              onClick={handleDelete}
              disabled={isPending}
              className="flex-1 bg-red-600 text-white hover:bg-red-700"
            >
              {isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              Excluir
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setConfirmingDelete(true)}
            disabled={isPending}
            className="border-red-300 text-red-700 hover:bg-red-50 hover:text-red-800"
          >
            <Trash2 className="h-4 w-4" />
            Excluir
          </Button>
          <Button
            type="submit"
            size="sm"
            disabled={isPending}
            className="flex-1 bg-slate-800 text-white hover:bg-slate-900"
          >
            {isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            Salvar
          </Button>
        </div>
      )}
    </form>
  )
}
