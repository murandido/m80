"use client"

import { useEffect, useMemo, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm, useWatch } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import {
  Eye,
  EyeOff,
  KeyRound,
  PhoneCall,
  Save,
  User as UserIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import {
  type UpdateMeRequest,
  type User,
  updateMeSchemaFor,
} from "@/schemas/user"
import { updateMeAction } from "@/actions/user"

export function ProfileForm({ user }: { user: User }) {
  const router = useRouter()
  const [showPasswords, setShowPasswords] = useState(false)
  const [saved, setSaved] = useState(false)
  const [isPending, startTransition] = useTransition()

  const schema = useMemo(() => updateMeSchemaFor(user.phone), [user.phone])

  const form = useForm<UpdateMeRequest>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: {
      name: user.name,
      phone: user.phone,
      new_password: "",
      current_password: "",
    },
  })

  // Re-seeded when the session user changes, which is what router.refresh() after a
  // save delivers - otherwise the form would keep comparing against the old phone and
  // keep asking for the password on a number that is already saved.
  useEffect(() => {
    form.reset({
      name: user.name,
      phone: user.phone,
      new_password: "",
      current_password: "",
    })
  }, [user]) // eslint-disable-line react-hooks/exhaustive-deps

  const phone = useWatch({ control: form.control, name: "phone" })
  const newPassword = useWatch({ control: form.control, name: "new_password" })
  const touchesLogin = newPassword !== "" || phone !== user.phone

  function onSubmit(data: UpdateMeRequest) {
    setSaved(false)
    startTransition(async () => {
      const result = await updateMeAction(data)
      if (result.error) {
        form.setError("root", { message: result.error })
        return
      }
      setSaved(true)
      setShowPasswords(false)
      router.refresh()
    })
  }

  const passwordToggle = (
    <InputGroupAddon align="inline-end">
      <InputGroupButton
        type="button"
        aria-label={showPasswords ? "Ocultar senhas" : "Mostrar senhas"}
        onClick={() => setShowPasswords((prev) => !prev)}
      >
        {showPasswords ? (
          <EyeOff className="h-4 w-4" />
        ) : (
          <Eye className="h-4 w-4" />
        )}
      </InputGroupButton>
    </InputGroupAddon>
  )

  return (
    <div className="rounded-md border border-slate-200 p-4">
      <div className="mb-4">
        <h2 className="text-sm font-semibold text-slate-800">
          Atualizar dados
        </h2>
        <p className="mt-0.5 text-xs text-slate-500">
          Para trocar a senha ou o telefone, confirme com a sua senha atual.
        </p>
      </div>

      <form onSubmit={form.handleSubmit(onSubmit)}>
        <Field className="flex flex-col gap-5">
          <div className="flex flex-col gap-1 text-sm">
            <Controller
              name="name"
              control={form.control}
              render={({ field, fieldState }) => (
                <>
                  <FieldLabel htmlFor="profile-name">
                    Nome <span className="text-destructive">*</span>
                  </FieldLabel>
                  <InputGroup className="bg-slate-200">
                    <InputGroupInput
                      {...field}
                      id="profile-name"
                      type="text"
                      aria-invalid={fieldState.invalid}
                      disabled={isPending}
                    />
                    <InputGroupAddon align="inline-end">
                      <UserIcon className="text-foreground h-4 w-4" />
                    </InputGroupAddon>
                  </InputGroup>
                  {fieldState.invalid && (
                    <FieldError errors={[fieldState.error]} />
                  )}
                </>
              )}
            />
          </div>

          <div className="flex flex-col gap-1 text-sm">
            <Controller
              name="phone"
              control={form.control}
              render={({ field, fieldState }) => (
                <>
                  <FieldLabel htmlFor="profile-phone">
                    Telefone <span className="text-destructive">*</span>
                  </FieldLabel>
                  <InputGroup className="bg-slate-200">
                    <InputGroupInput
                      {...field}
                      id="profile-phone"
                      type="tel"
                      inputMode="numeric"
                      aria-invalid={fieldState.invalid}
                      placeholder="11999999999"
                      disabled={isPending}
                    />
                    <InputGroupAddon align="inline-end">
                      <PhoneCall className="text-foreground h-4 w-4" />
                    </InputGroupAddon>
                  </InputGroup>
                  <FieldDescription className="text-xs">
                    É com este número que você entra no painel e que o bot
                    reconhece as suas mensagens no WhatsApp.
                  </FieldDescription>
                  {fieldState.invalid && (
                    <FieldError errors={[fieldState.error]} />
                  )}
                </>
              )}
            />
          </div>

          <div className="flex flex-col gap-1 text-sm">
            <Controller
              name="new_password"
              control={form.control}
              render={({ field, fieldState }) => (
                <>
                  <FieldLabel htmlFor="profile-new-password">
                    Nova senha
                  </FieldLabel>
                  <InputGroup className="bg-slate-200">
                    <InputGroupInput
                      {...field}
                      id="profile-new-password"
                      type={showPasswords ? "text" : "password"}
                      autoComplete="new-password"
                      aria-invalid={fieldState.invalid}
                      placeholder="Deixe em branco para não alterar"
                      disabled={isPending}
                    />
                    {passwordToggle}
                  </InputGroup>
                  {fieldState.invalid && (
                    <FieldError errors={[fieldState.error]} />
                  )}
                </>
              )}
            />
          </div>

          {touchesLogin && (
            <div className="flex flex-col gap-1 text-sm">
              <Controller
                name="current_password"
                control={form.control}
                render={({ field, fieldState }) => (
                  <>
                    <FieldLabel htmlFor="profile-current-password">
                      Senha atual <span className="text-destructive">*</span>
                    </FieldLabel>
                    <InputGroup className="bg-slate-200">
                      <InputGroupInput
                        {...field}
                        id="profile-current-password"
                        type={showPasswords ? "text" : "password"}
                        autoComplete="current-password"
                        aria-invalid={fieldState.invalid}
                        disabled={isPending}
                      />
                      <InputGroupAddon align="inline-end">
                        <KeyRound className="text-foreground h-4 w-4" />
                      </InputGroupAddon>
                    </InputGroup>
                    {fieldState.invalid && (
                      <FieldError errors={[fieldState.error]} />
                    )}
                  </>
                )}
              />
            </div>
          )}

          <Separator />

          {form.formState.errors.root && (
            <p className="text-destructive text-sm">
              {form.formState.errors.root.message}
            </p>
          )}
          {saved && !form.formState.isDirty && (
            <p className="text-sm text-emerald-700">Dados atualizados.</p>
          )}

          <Button
            type="submit"
            className="h-full bg-slate-800 hover:bg-slate-900"
            disabled={isPending}
          >
            <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
              {isPending ? <Spinner /> : <Save className="h-4 w-4" />}
              {isPending ? "Salvando..." : "Salvar alterações"}
            </div>
          </Button>
        </Field>
      </form>
    </div>
  )
}
