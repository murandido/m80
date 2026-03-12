"use client"

import { Separator } from "@/components/ui/separator"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { KeyRound, PhoneCall } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Controller, useForm } from "react-hook-form"
import { z } from "zod"
import { loginSchema } from "@/schemas/auth"
import { zodResolver } from "@hookform/resolvers/zod"
import { useTransition } from "react"
import { loginAction } from "@/actions/auth"
import { Spinner } from "@/components/ui/spinner"

export function LoginForm() {
  const [isPending, startTransition] = useTransition()

  const form = useForm<z.infer<typeof loginSchema.request>>({
    resolver: zodResolver(loginSchema.request),
    mode: "onTouched",
    defaultValues: {
      phone: "",
      password: "",
    },
  })

  function onSubmit(data: z.infer<typeof loginSchema.request>) {
    startTransition(async () => {
      const result = await loginAction(null, data)

      if (result.error) {
        form.setError("phone", { message: "Telefone e/ou senha inválidos!" })
        form.setError("password", { message: "Telefone e/ou senha inválidos!" })
      }
    })
  }

  return (
    <>
      {/* header */}
      <div className="flex flex-col items-center justify-center gap-1">
        <h1 className="text-2xl font-semibold">Seja Bem Vindo!</h1>
        <p className="text-center text-slate-900">
          Para prosseguir, é necessário estar previamente cadastrado por um
          admin
        </p>
      </div>
      <Separator />
      {/* form */}
      <form className="w-full" onSubmit={form.handleSubmit(onSubmit)}>
        <Field className="flex flex-col gap-6">
          {/* phone input */}
          <div className="flex flex-col gap-1 text-sm">
            <Controller
              name="phone"
              control={form.control}
              render={({ field, fieldState }) => (
                <>
                  <FieldLabel htmlFor="phone">
                    Número de Telefone{" "}
                    <span className="text-destructive">*</span>
                  </FieldLabel>
                  <InputGroup className="bg-slate-200">
                    <InputGroupInput
                      {...field}
                      id="phone"
                      type="tel"
                      aria-invalid={fieldState.invalid}
                      pattern="[0-9]{10,11}"
                      placeholder="11999999999"
                      required
                      disabled={isPending}
                    />
                    <InputGroupAddon align="inline-end">
                      <PhoneCall className="text-foreground" />
                    </InputGroupAddon>
                  </InputGroup>
                  {fieldState.invalid && (
                    <FieldError errors={[fieldState.error]} />
                  )}
                </>
              )}
            />
          </div>
          {/* password input */}
          <div className="flex flex-col gap-1">
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
                      type="password"
                      aria-invalid={fieldState.invalid}
                      placeholder="5ua-Senh@-Aqu1"
                      required
                      disabled={isPending}
                    />
                    <InputGroupAddon align="inline-end">
                      <KeyRound className="text-foreground" />
                    </InputGroupAddon>
                  </InputGroup>
                  {fieldState.invalid && (
                    <FieldError errors={[fieldState.error]} />
                  )}
                </>
              )}
            />
          </div>
          <Button
            type="submit"
            size="sm"
            className="bg-slate-700"
            disabled={isPending}
          >
            {isPending ? <Spinner /> : "Entrar"}
          </Button>
        </Field>
      </form>
    </>
  )
}
