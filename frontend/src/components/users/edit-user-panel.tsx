"use client"

import { useEffect, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm, useWatch } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import {
  Eye,
  EyeOff,
  Hash,
  KeyRound,
  PhoneCall,
  User as UserIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { User, updateUserSchema } from "@/schemas/user"
import { RoleSelector } from "@/components/users/role-selector"
import { UserAgenciesField } from "@/components/users/user-agencies-field"
import { updateUserAction } from "@/actions/user"
import type { Agency } from "@/schemas/agency"

type FormData = z.infer<typeof updateUserSchema.request>

interface EditUserPanelProps {
  user: User
  isOpen: boolean
  onSaved: () => void
  // See CreateUserForm: agencies and the dev role are a dev's alone.
  viewerIsDev: boolean
  agencies: Agency[]
}

export function EditUserPanel({ user, isOpen, onSaved, viewerIsDev, agencies }: EditUserPanelProps) {
  const router = useRouter()
  const [showPassword, setShowPassword] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(updateUserSchema.request),
    mode: "onTouched",
    defaultValues: {
      name: user.name,
      phone: user.phone,
      password: "",
      role: user.role,
      agency_ids: viewerIsDev ? user.agency_ids : undefined,
    },
  })
  const role = useWatch({ control: form.control, name: "role" })

  // Re-seeded whenever the panel closes, and whenever the record itself changes
  // while closed. The second half matters after a save or a merge: the panel closes
  // before router.refresh() lands, so resetting only on close would reopen showing
  // the pre-save values - and saving that form again would write them back.
  useEffect(() => {
    if (!isOpen) {
      form.reset({
        name: user.name,
        phone: user.phone,
        password: "",
        role: user.role,
        agency_ids: viewerIsDev ? user.agency_ids : undefined,
      })
      // eslint-disable-next-line react-hooks/set-state-in-effect -- resets the panel each time it opens for a user
      setShowPassword(false)
    }
  }, [isOpen, user]) // eslint-disable-line react-hooks/exhaustive-deps

  function onSubmit(data: FormData) {
    startTransition(async () => {
      const result = await updateUserAction(user.id, data)
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
              Informações do usuário
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Veja e/ou atualize as informações do usuário aqui!
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
                    value={user.id}
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

              {/* phone */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="phone"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-phone">
                        Telefone <span className="text-destructive">*</span>
                      </FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-phone"
                          type="tel"
                          aria-invalid={fieldState.invalid}
                          pattern="[0-9]{10,11}"
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

              {/* new password */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="password"
                  control={form.control}
                  render={({ field, fieldState }) => (
                    <>
                      <FieldLabel htmlFor="edit-password">Nova senha</FieldLabel>
                      <InputGroup className="bg-slate-200">
                        <InputGroupInput
                          {...field}
                          id="edit-password"
                          type={showPassword ? "text" : "password"}
                          aria-invalid={fieldState.invalid}
                          placeholder="Deixe em branco para não alterar"
                          disabled={isPending}
                        />
                        <InputGroupAddon align="inline-end">
                          <InputGroupButton
                            type="button"
                            onClick={() => setShowPassword((prev) => !prev)}
                          >
                            {showPassword ? (
                              <EyeOff className="h-4 w-4" />
                            ) : (
                              <Eye className="h-4 w-4" />
                            )}
                          </InputGroupButton>
                        </InputGroupAddon>
                      </InputGroup>
                      {fieldState.invalid && (
                        <FieldError errors={[fieldState.error]} />
                      )}
                    </>
                  )}
                />
              </div>

              {/* role */}
              <div className="flex flex-col gap-1 text-sm">
                <Controller
                  name="role"
                  control={form.control}
                  render={({ field }) => (
                    <RoleSelector
                      value={field.value}
                      onChange={field.onChange}
                      disabled={isPending}
                      allowDev={viewerIsDev}
                    />
                  )}
                />
              </div>

              {/* agencies - dev only, and meaningless for a dev account */}
              {viewerIsDev && role !== "dev" && (
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="agency_ids"
                    control={form.control}
                    render={({ field }) => (
                      <UserAgenciesField
                        agencies={agencies}
                        value={field.value ?? []}
                        onChange={field.onChange}
                        disabled={isPending}
                      />
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

              <Button
                type="submit"
                className="h-full bg-slate-800 hover:bg-slate-900"
                disabled={isPending}
              >
                <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
                  {isPending ? <Spinner /> : <KeyRound className="h-4 w-4" />}
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
