"use client"

import { useState, useTransition } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { CheckCircle2, Eye, EyeOff } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"
import { changeOwnPasswordAction } from "@/actions/portal"
import { changeOwnPasswordSchema, type ChangeOwnPasswordRequest } from "@/schemas/portal"

const FIELDS: { name: keyof ChangeOwnPasswordRequest; label: string; autoComplete: string }[] = [
  { name: "current_password", label: "Senha atual", autoComplete: "current-password" },
  { name: "new_password", label: "Nova senha", autoComplete: "new-password" },
  { name: "confirm_password", label: "Repita a nova senha", autoComplete: "new-password" },
]

// The statement client's own M80 password. Deliberately not the staff profile form: a client
// has no name or phone to edit here, since the phone is the login the team registered.
export function PortalPasswordForm() {
  const [showPasswords, setShowPasswords] = useState(false)
  const [saved, setSaved] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<ChangeOwnPasswordRequest>({
    resolver: zodResolver(changeOwnPasswordSchema),
    mode: "onTouched",
    defaultValues: { current_password: "", new_password: "", confirm_password: "" },
  })

  function onSubmit(data: ChangeOwnPasswordRequest) {
    setSaved(false)
    startTransition(async () => {
      const result = await changeOwnPasswordAction(data)
      if ("error" in result) {
        form.setError("root", { message: result.error })
        return
      }
      form.reset()
      setShowPasswords(false)
      setSaved(true)
    })
  }

  return (
    <form
      onSubmit={form.handleSubmit(onSubmit)}
      className="flex flex-col gap-5 rounded-md border border-slate-200 p-4"
    >
      {FIELDS.map(({ name, label, autoComplete }) => (
        <Field key={name} className="flex flex-col gap-1 text-sm">
          <Controller
            name={name}
            control={form.control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={`portal-${name}`}>{label}</FieldLabel>
                <InputGroup>
                  <InputGroupInput
                    {...field}
                    id={`portal-${name}`}
                    type={showPasswords ? "text" : "password"}
                    autoComplete={autoComplete}
                    aria-invalid={fieldState.invalid}
                    disabled={isPending}
                    onChange={(event) => {
                      setSaved(false)
                      field.onChange(event)
                    }}
                  />
                  <InputGroupAddon align="inline-end">
                    <InputGroupButton
                      type="button"
                      aria-label={showPasswords ? "Ocultar senhas" : "Mostrar senhas"}
                      onClick={() => setShowPasswords((prev) => !prev)}
                    >
                      {showPasswords ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </InputGroupButton>
                  </InputGroupAddon>
                </InputGroup>
                {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
              </>
            )}
          />
        </Field>
      ))}

      {form.formState.errors.root && (
        <p className="text-destructive text-sm">{form.formState.errors.root.message}</p>
      )}
      {saved && (
        <p className="flex items-center gap-1.5 text-sm text-emerald-700">
          <CheckCircle2 className="h-4 w-4" />
          Senha trocada. Use a nova no próximo acesso.
        </p>
      )}

      <div>
        <Button type="submit" disabled={isPending}>
          {isPending && <Spinner />}
          Trocar senha
        </Button>
      </div>
    </form>
  )
}
