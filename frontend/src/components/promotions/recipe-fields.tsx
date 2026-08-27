"use client"

import { Controller, type UseFormReturn } from "react-hook-form"
import { Globe, Tag, Timer } from "lucide-react"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"

// The recipe form has eleven fields and is rendered identically when creating and when
// editing, so it lives here rather than being written out twice. The rest of the
// cadastro screens duplicate their forms, but they carry three or four fields each -
// duplicating this one would mean two places to keep the selector help text honest.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type RecipeForm = UseFormReturn<any>

interface RecipeFieldsProps {
  form: RecipeForm
  programs: LoyaltyProgram[]
  isPending: boolean
  idPrefix: string
}

export function RecipeFields({ form, programs, isPending, idPrefix }: RecipeFieldsProps) {
  const id = (name: string) => `${idPrefix}-${name}`

  return (
    <Field className="flex flex-col gap-5">
      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name="name"
          control={form.control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={id("name")}>
                Nome <span className="text-destructive">*</span>
              </FieldLabel>
              <InputGroup className="bg-slate-200">
                <InputGroupInput
                  {...field}
                  id={id("name")}
                  aria-invalid={fieldState.invalid}
                  placeholder="Azul Fidelidade"
                  disabled={isPending}
                />
                <InputGroupAddon align="inline-end">
                  <Tag className="text-foreground h-4 w-4" />
                </InputGroupAddon>
              </InputGroup>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name="domain"
          control={form.control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={id("domain")}>
                Domínio <span className="text-destructive">*</span>
              </FieldLabel>
              <InputGroup className="bg-slate-200">
                <InputGroupInput
                  {...field}
                  id={id("domain")}
                  aria-invalid={fieldState.invalid}
                  placeholder="voeazul.com.br"
                  disabled={isPending}
                />
                <InputGroupAddon align="inline-end">
                  <Globe className="text-foreground h-4 w-4" />
                </InputGroupAddon>
              </InputGroup>
              <p className="text-xs text-slate-500">
                Sem https:// e sem www. Vale para os subdomínios também.
              </p>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name="loyalty_program_id"
          control={form.control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={id("program")}>
                Programa de fidelidade <span className="text-destructive">*</span>
              </FieldLabel>
              <LoyaltyProgramCombobox
                loyaltyPrograms={programs}
                value={field.value}
                onChange={field.onChange}
                disabled={isPending}
              />
              <p className="text-xs text-slate-500">
                Define quais CPFs entram nessa promoção.
              </p>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      <Separator />

      <div className="flex flex-col gap-1">
        <p className="text-sm font-medium text-slate-800">Seletores</p>
        <p className="text-xs text-slate-500">
          Deixe em branco para o bot descobrir sozinho. Preencha só o que ele errar —
          use o botão <span className="font-medium">Testar</span> para ver o que ele
          encontra na página.
        </p>
      </div>

      {(
        [
          {
            name: "cpf_selector",
            label: "Campo de CPF",
            placeholder: "#cpf",
          },
          {
            name: "checkbox_selector",
            label: "Aceite das regras",
            placeholder: 'input[name="check"]',
          },
          {
            name: "submit_selector",
            label: "Botão de enviar",
            placeholder: "button.participar",
          },
        ] as const
      ).map((selector) => (
        <div key={selector.name} className="flex flex-col gap-1 text-sm">
          <Controller
            name={selector.name}
            control={form.control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={id(selector.name)}>{selector.label}</FieldLabel>
                <InputGroup className="bg-slate-200">
                  <InputGroupInput
                    {...field}
                    id={id(selector.name)}
                    aria-invalid={fieldState.invalid}
                    placeholder={selector.placeholder}
                    disabled={isPending}
                  />
                </InputGroup>
                {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
              </>
            )}
          />
        </div>
      ))}

      <Separator />

      <div className="flex flex-col gap-1">
        <p className="text-sm font-medium text-slate-800">Como reconhecer a resposta</p>
        <p className="text-xs text-slate-500">
          Expressões regulares comparadas com o texto que o site mostra depois de
          enviar. Em branco, valem as expressões padrão.
        </p>
      </div>

      {(
        [
          {
            name: "success_pattern",
            label: "Sucesso",
            placeholder: "solicitação enviada",
          },
          {
            name: "failure_pattern",
            label: "Falha",
            placeholder: "cpf inválido",
          },
        ] as const
      ).map((pattern) => (
        <div key={pattern.name} className="flex flex-col gap-1 text-sm">
          <Controller
            name={pattern.name}
            control={form.control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={id(pattern.name)}>{pattern.label}</FieldLabel>
                <InputGroup className="bg-slate-200">
                  <InputGroupInput
                    {...field}
                    id={id(pattern.name)}
                    aria-invalid={fieldState.invalid}
                    placeholder={pattern.placeholder}
                    disabled={isPending}
                  />
                </InputGroup>
                {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
              </>
            )}
          />
        </div>
      ))}

      <Separator />

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name="wait_seconds"
          control={form.control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={id("wait")}>Espera antes de ler a página</FieldLabel>
              <InputGroup className="bg-slate-200">
                <InputGroupInput
                  {...field}
                  id={id("wait")}
                  inputMode="numeric"
                  aria-invalid={fieldState.invalid}
                  placeholder="5"
                  disabled={isPending}
                />
                <InputGroupAddon align="inline-end">
                  <Timer className="text-foreground h-4 w-4" />
                </InputGroupAddon>
              </InputGroup>
              <p className="text-xs text-slate-500">
                Segundos. Aumente quando o formulário demorar a aparecer na tela.
              </p>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name="notes"
          control={form.control}
          render={({ field }) => (
            <>
              <FieldLabel htmlFor={id("notes")}>Observações</FieldLabel>
              <Textarea
                {...field}
                id={id("notes")}
                rows={2}
                className="bg-slate-200"
                placeholder="Particularidades desse site"
                disabled={isPending}
              />
            </>
          )}
        />
      </div>

      <div className="flex items-center justify-between text-sm">
        <div className="flex flex-col">
          <FieldLabel htmlFor={id("active")}>Ativa</FieldLabel>
          <p className="text-xs text-slate-500">
            Desativada, um link desse site não dispara cadastro nenhum.
          </p>
        </div>
        <Controller
          name="active"
          control={form.control}
          render={({ field }) => (
            <Switch
              id={id("active")}
              checked={field.value}
              onCheckedChange={field.onChange}
              disabled={isPending}
            />
          )}
        />
      </div>
    </Field>
  )
}
