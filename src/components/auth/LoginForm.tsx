import { Separator } from "@/components/ui/separator"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { KeyRound, PhoneCall } from "lucide-react"
import { Button } from "@/components/ui/button"

export function LoginForm() {
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
      <Field className="flex flex-col gap-6">
        <div className="flex flex-col gap-1 text-sm">
          <FieldLabel htmlFor="phone">
            Número de Telefone <span className="text-destructive">*</span>
          </FieldLabel>
          <InputGroup className="bg-slate-200">
            <InputGroupInput
              id="phone"
              type="tel"
              placeholder="(11) 99999-9999"
              name="phone"
              required
            />
            <InputGroupAddon align="inline-end">
              <PhoneCall className="text-foreground" />
            </InputGroupAddon>
          </InputGroup>
        </div>
        <div className="flex flex-col gap-1">
          <FieldLabel htmlFor="password">
            Senha <span className="text-destructive">*</span>
          </FieldLabel>
          <InputGroup className="bg-slate-200">
            <InputGroupInput
              id="phone"
              type="password"
              placeholder="5ua-Senh@-Aqu1"
              name="phone"
              required
            />
            <InputGroupAddon align="inline-end">
              <KeyRound className="text-foreground" />
            </InputGroupAddon>
          </InputGroup>
        </div>
        <Button size="sm" className="bg-slate-700">
          Entrar
        </Button>
      </Field>
    </>
  )
}
