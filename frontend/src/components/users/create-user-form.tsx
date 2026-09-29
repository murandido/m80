"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Controller, useForm, useWatch } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { UserCog, X, Eye, EyeOff, PhoneCall, User, KeyRound, Save } from "lucide-react"
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
import { createUserSchema } from "@/schemas/user"
import { RoleSelector } from "@/components/users/role-selector"
import { UserAgenciesField } from "@/components/users/user-agencies-field"
import { createUserAction } from "@/actions/user"
import type { Agency } from "@/schemas/agency"

type FormData = z.infer<typeof createUserSchema.request>

interface CreateUserFormProps {
  // Whether the person filling the form is a dev: only then are agencies and the dev role
  // offered, since the API refuses both from anyone else.
  viewerIsDev: boolean
  agencies: Agency[]
}

export function CreateUserForm({ viewerIsDev, agencies }: CreateUserFormProps) {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [isPending, startTransition] = useTransition()

  const form = useForm<FormData>({
    resolver: zodResolver(createUserSchema.request),
    mode: "onTouched",
    defaultValues: {
      name: "",
      phone: "",
      password: "",
      role: "user",
      agency_ids: viewerIsDev ? [] : undefined,
    },
  })
  const role = useWatch({ control: form.control, name: "role" })

  function handleToggle() {
    if (isOpen) {
      form.reset()
      setShowPassword(false)
    }
    setIsOpen((prev) => !prev)
  }

  function onSubmit(data: FormData) {
    startTransition(async () => {
      // A dev's role covers every agency; grants on one would only go stale.
      const payload = data.role === "dev" ? { ...data, agency_ids: undefined } : data
      const result = await createUserAction(payload)

      if (result.error) {
        form.setError("root", { message: result.error })
        return
      }

      form.reset()
      setShowPassword(false)
      setIsOpen(false)
      router.refresh()
    })
  }

  return (
    <div className="flex flex-col gap-4">
      {/* toggle button */}
      <Button
        type="button"
        onClick={handleToggle}
        className={cn(
          "h-full transition-colors duration-200",
          isOpen ? "bg-slate-500 hover:bg-slate-600" : "bg-slate-800 hover:bg-slate-900",
        )}
      >
        <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
          {isOpen ? <X className="h-4 w-4" /> : <UserCog className="h-4 w-4" />}
          {isOpen ? "Cancelar Cadastro" : "Cadastrar Usuário"}
        </div>
      </Button>

      {/* animated form panel */}
      <div
        className={cn(
          "grid transition-all duration-300 ease-in-out",
          isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}
      >
        <div className="overflow-hidden">
          <div className="rounded-md border border-slate-200 p-4">
            <form onSubmit={form.handleSubmit(onSubmit)}>
              <Field className="flex flex-col gap-5">
                {/* name */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="name"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="name">
                          Nome <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="name"
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
                        <FieldLabel htmlFor="phone">
                          Telefone <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="phone"
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

                {/* password */}
                <div className="flex flex-col gap-1 text-sm">
                  <Controller
                    name="password"
                    control={form.control}
                    render={({ field, fieldState }) => (
                      <>
                        <FieldLabel htmlFor="password">
                          Senha <span className="text-destructive">*</span>
                        </FieldLabel>
                        <InputGroup className="bg-slate-200">
                          <InputGroupInput
                            {...field}
                            id="password"
                            type={showPassword ? "text" : "password"}
                            aria-invalid={fieldState.invalid}
                            placeholder="5ua-Senh@-Aqu1"
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

                {/* root error */}
                {form.formState.errors.root && (
                  <p className="text-destructive text-sm">
                    {form.formState.errors.root.message}
                  </p>
                )}

                {/* submit */}
                <Button
                  type="submit"
                  className="h-full bg-slate-800 hover:bg-slate-900"
                  disabled={isPending}
                >
                  <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
                    {isPending ? <Spinner /> : <Save className="h-4 w-4" />}
                    {isPending ? "Salvando..." : "Salvar"}
                  </div>
                </Button>
              </Field>
            </form>
          </div>
        </div>
      </div>
    </div>
  )
}
