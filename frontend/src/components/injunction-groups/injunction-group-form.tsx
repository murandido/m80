"use client"

import { useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useFieldArray, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupInput } from "@/components/ui/input-group"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { saveInjunctionGroupAction } from "@/actions/injunction-group"
import { injunctionGroupFormSchema } from "@/schemas/injunction-group"
import type { InjunctionAccountOption, InjunctionGroup, InjunctionGroupForm } from "@/schemas/injunction-group"

type Options = {
  accounts: InjunctionAccountOption[]
  users: { id: string; name: string }[]
  agencies: { id: string; name: string }[]
}

function TextField({
  id,
  label,
  placeholder,
  error,
  disabled,
  ...field
}: {
  id: string
  label: string
  placeholder?: string
  error?: { message?: string }
  disabled: boolean
  value: string
  onChange: (value: string) => void
  onBlur: () => void
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <InputGroup className="bg-slate-200">
        <InputGroupInput
          id={id}
          value={field.value}
          onChange={(event) => field.onChange(event.target.value)}
          onBlur={field.onBlur}
          placeholder={placeholder}
          aria-invalid={Boolean(error)}
          disabled={disabled}
        />
      </InputGroup>
      {error && <FieldError errors={[error]} />}
    </Field>
  )
}

// Creates or edits a Tropa: which injunction accounts its workbook covers (by the label in the
// Conta column), which panel user each Emissor name is, the WhatsApp group and the link.
export function InjunctionGroupFormPanel({
  group,
  options,
  onDone,
}: {
  group: InjunctionGroup | null
  options: Options
  onDone: () => void
}) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  const form = useForm<InjunctionGroupForm>({
    resolver: zodResolver(injunctionGroupFormSchema),
    defaultValues: {
      name: group?.name ?? "",
      agency_id: group?.agency_id ?? options.agencies[0]?.id ?? "",
      whatsapp_chat_name: group?.whatsapp_chat_name ?? "",
      spreadsheet_url: "",
      accounts: group?.accounts.map((a) => ({ account_label: a.account_label, account_id: a.account_id })) ?? [],
      issuers: group?.issuers.map((i) => ({ issuer_name: i.issuer_name, user_id: i.user_id })) ?? [],
    },
  })
  const accounts = useFieldArray({ control: form.control, name: "accounts" })
  const issuers = useFieldArray({ control: form.control, name: "issuers" })

  function onSubmit(data: InjunctionGroupForm) {
    startTransition(async () => {
      const result = await saveInjunctionGroupAction(group?.id ?? null, data)
      if ("error" in result) {
        form.setError("root", { message: result.error })
        return
      }
      toast.success(group ? "Tropa atualizada." : "Tropa criada.")
      onDone()
      router.refresh()
    })
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4 rounded-md border p-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <Controller
          name="name"
          control={form.control}
          render={({ field, fieldState }) => (
            <TextField id="tropa-name" label="Nome" placeholder="Tropa Azul 1" error={fieldState.error} disabled={isPending} {...field} />
          )}
        />
        <Controller
          name="agency_id"
          control={form.control}
          render={({ field, fieldState }) => (
            <Field>
              <FieldLabel htmlFor="tropa-agency">Agência</FieldLabel>
              <Select value={field.value} onValueChange={field.onChange} disabled={isPending}>
                <SelectTrigger id="tropa-agency" className="bg-slate-200">
                  <SelectValue placeholder="Escolha" />
                </SelectTrigger>
                <SelectContent>
                  {options.agencies.map((agency) => (
                    <SelectItem key={agency.id} value={agency.id}>
                      {agency.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {fieldState.error && <FieldError errors={[fieldState.error]} />}
            </Field>
          )}
        />
        <Controller
          name="whatsapp_chat_name"
          control={form.control}
          render={({ field, fieldState }) => (
            <TextField
              id="tropa-chat"
              label="Grupo do WhatsApp (nome exato)"
              placeholder="Tropa Azul 1"
              error={fieldState.error}
              disabled={isPending}
              {...field}
            />
          )}
        />
        <Controller
          name="spreadsheet_url"
          control={form.control}
          render={({ field, fieldState }) => (
            <TextField
              id="tropa-sheet"
              label={group?.has_spreadsheet ? `Link da planilha (atual: ${group.spreadsheet_hint}; vazio mantém)` : "Link da planilha no OneDrive"}
              placeholder="https://onedrive.live.com/..."
              error={fieldState.error}
              disabled={isPending}
              {...field}
            />
          )}
        />
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Contas liminares (coluna Conta da planilha)</span>
          <Button type="button" variant="outline" size="sm" onClick={() => accounts.append({ account_label: "", account_id: "" })}>
            <Plus className="h-4 w-4" /> Conta
          </Button>
        </div>
        {accounts.fields.length === 0 && (
          <p className="text-xs text-muted-foreground">Sem contas: nenhuma linha da planilha será lida.</p>
        )}
        {accounts.fields.map((item, index) => (
          <div key={item.id} className="grid items-end gap-2 sm:grid-cols-[1fr_2fr_auto]">
            <Controller
              name={`accounts.${index}.account_label`}
              control={form.control}
              render={({ field, fieldState }) => (
                <TextField id={`acc-label-${index}`} label="Rótulo" placeholder="HUDSON" error={fieldState.error} disabled={isPending} {...field} />
              )}
            />
            <Controller
              name={`accounts.${index}.account_id`}
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel>Conta</FieldLabel>
                  <Select value={field.value} onValueChange={field.onChange} disabled={isPending}>
                    <SelectTrigger className="bg-slate-200">
                      <SelectValue placeholder="Conta liminar" />
                    </SelectTrigger>
                    <SelectContent>
                      {options.accounts.map((account) => (
                        <SelectItem key={account.id} value={account.id}>
                          {account.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {fieldState.error && <FieldError errors={[fieldState.error]} />}
                </Field>
              )}
            />
            <Button type="button" variant="ghost" size="sm" onClick={() => accounts.remove(index)} aria-label="Remover conta">
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Emissores (coluna Emissor da planilha)</span>
          <Button type="button" variant="outline" size="sm" onClick={() => issuers.append({ issuer_name: "", user_id: "" })}>
            <Plus className="h-4 w-4" /> Emissor
          </Button>
        </div>
        {issuers.fields.map((item, index) => (
          <div key={item.id} className="grid items-end gap-2 sm:grid-cols-[1fr_2fr_auto]">
            <Controller
              name={`issuers.${index}.issuer_name`}
              control={form.control}
              render={({ field, fieldState }) => (
                <TextField id={`iss-name-${index}`} label="Nome" placeholder="PEDRO" error={fieldState.error} disabled={isPending} {...field} />
              )}
            />
            <Controller
              name={`issuers.${index}.user_id`}
              control={form.control}
              render={({ field, fieldState }) => (
                <Field>
                  <FieldLabel>Usuário</FieldLabel>
                  <Select value={field.value} onValueChange={field.onChange} disabled={isPending}>
                    <SelectTrigger className="bg-slate-200">
                      <SelectValue placeholder="Usuário do painel" />
                    </SelectTrigger>
                    <SelectContent>
                      {options.users.map((user) => (
                        <SelectItem key={user.id} value={user.id}>
                          {user.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {fieldState.error && <FieldError errors={[fieldState.error]} />}
                </Field>
              )}
            />
            <Button type="button" variant="ghost" size="sm" onClick={() => issuers.remove(index)} aria-label="Remover emissor">
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>

      <div className="flex items-center gap-3">
        <Button type="submit" disabled={isPending}>
          {group ? "Salvar" : "Criar Tropa"}
        </Button>
        <Button type="button" variant="outline" disabled={isPending} onClick={onDone}>
          Cancelar
        </Button>
        {form.formState.errors.root && <p className="text-sm text-destructive">{form.formState.errors.root.message}</p>}
      </div>
    </form>
  )
}
