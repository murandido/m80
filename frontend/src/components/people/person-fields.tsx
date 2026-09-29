"use client"

import {
  Controller,
  type Control,
  type FieldValues,
  type Path,
} from "react-hook-form"
import {
  User,
  Hash,
  Mail,
  PhoneCall,
  MapPin,
  Landmark,
  Building2,
  Home,
  Signpost,
  FileText,
} from "lucide-react"
import { FieldError, FieldLabel } from "@/components/ui/field"
import { DateInput } from "@/components/ui/date-input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"

interface PersonFieldsProps<T extends FieldValues> {
  control: Control<T>
  // Where the person's fields sit inside the surrounding form: "" when the form is the
  // person, "holder." when the person is the holder of the account being created.
  namePrefix?: string
  // Keeps the ids unique when two of these could be on the page at once.
  idPrefix?: string
  disabled?: boolean
}

// The personal-data and address fields of a person, shared by every form that registers one.
// The names are built from a prefix, so react-hook-form cannot type-check them against T -
// the caller's schema is what guarantees they exist.
export function PersonFields<T extends FieldValues>({
  control,
  namePrefix = "",
  idPrefix = "",
  disabled = false,
}: PersonFieldsProps<T>) {
  return (
    <>
      <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
        Dados pessoais
      </h3>

      {/* name */}
      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={`${namePrefix}name` as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}name`}>
                Nome <span className="text-destructive">*</span>
              </FieldLabel>
              <InputGroup className="bg-slate-200">
                <InputGroupInput
                  {...field}
                  id={`${idPrefix}name`}
                  type="text"
                  aria-invalid={fieldState.invalid}
                  placeholder="João Silva"
                  disabled={disabled}
                />
                <InputGroupAddon align="inline-end">
                  <User className="text-foreground h-4 w-4" />
                </InputGroupAddon>
              </InputGroup>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      {/* cpf */}
      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={`${namePrefix}cpf` as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}cpf`}>CPF</FieldLabel>
              <InputGroup className="bg-slate-200">
                <InputGroupInput
                  {...field}
                  id={`${idPrefix}cpf`}
                  type="text"
                  aria-invalid={fieldState.invalid}
                  placeholder="000.000.000-00"
                  disabled={disabled}
                />
                <InputGroupAddon align="inline-end">
                  <Hash className="text-foreground h-4 w-4" />
                </InputGroupAddon>
              </InputGroup>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      {/* birth_date */}
      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={`${namePrefix}birth_date` as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}birth_date`}>
                Data de nascimento
              </FieldLabel>
              <DateInput
                id={`${idPrefix}birth_date`}
                value={field.value ?? ""}
                onChange={field.onChange}
                aria-invalid={fieldState.invalid}
                disabled={disabled}
                className="[&_input]:bg-slate-200"
              />
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      {/* email */}
      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={`${namePrefix}email` as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}email`}>Email</FieldLabel>
              <InputGroup className="bg-slate-200">
                <InputGroupInput
                  {...field}
                  id={`${idPrefix}email`}
                  type="email"
                  aria-invalid={fieldState.invalid}
                  placeholder="joao@email.com"
                  disabled={disabled}
                />
                <InputGroupAddon align="inline-end">
                  <Mail className="text-foreground h-4 w-4" />
                </InputGroupAddon>
              </InputGroup>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      {/* phone */}
      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={`${namePrefix}phone` as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}phone`}>Telefone</FieldLabel>
              <InputGroup className="bg-slate-200">
                <InputGroupInput
                  {...field}
                  id={`${idPrefix}phone`}
                  type="tel"
                  aria-invalid={fieldState.invalid}
                  placeholder="11999999999"
                  disabled={disabled}
                />
                <InputGroupAddon align="inline-end">
                  <PhoneCall className="text-foreground h-4 w-4" />
                </InputGroupAddon>
              </InputGroup>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      <Separator />
      <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
        Endereço
      </h3>

      {/* zip_code */}
      <div className="flex flex-col gap-1 text-sm">
        <Controller
          name={`${namePrefix}zip_code` as Path<T>}
          control={control}
          render={({ field, fieldState }) => (
            <>
              <FieldLabel htmlFor={`${idPrefix}zip_code`}>CEP</FieldLabel>
              <InputGroup className="bg-slate-200">
                <InputGroupInput
                  {...field}
                  id={`${idPrefix}zip_code`}
                  type="text"
                  aria-invalid={fieldState.invalid}
                  placeholder="00000-000"
                  disabled={disabled}
                />
                <InputGroupAddon align="inline-end">
                  <MapPin className="text-foreground h-4 w-4" />
                </InputGroupAddon>
              </InputGroup>
              {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
            </>
          )}
        />
      </div>

      {/* state + city */}
      <div className="grid grid-cols-2 gap-3">
        <div className="flex flex-col gap-1 text-sm">
          <Controller
            name={`${namePrefix}state` as Path<T>}
            control={control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={`${idPrefix}state`}>Estado</FieldLabel>
                <InputGroup className="bg-slate-200">
                  <InputGroupInput
                    {...field}
                    id={`${idPrefix}state`}
                    type="text"
                    aria-invalid={fieldState.invalid}
                    placeholder="SP"
                    disabled={disabled}
                  />
                  <InputGroupAddon align="inline-end">
                    <Landmark className="text-foreground h-4 w-4" />
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
            name={`${namePrefix}city` as Path<T>}
            control={control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={`${idPrefix}city`}>Cidade</FieldLabel>
                <InputGroup className="bg-slate-200">
                  <InputGroupInput
                    {...field}
                    id={`${idPrefix}city`}
                    type="text"
                    aria-invalid={fieldState.invalid}
                    placeholder="São Paulo"
                    disabled={disabled}
                  />
                  <InputGroupAddon align="inline-end">
                    <Building2 className="text-foreground h-4 w-4" />
                  </InputGroupAddon>
                </InputGroup>
                {fieldState.invalid && (
                  <FieldError errors={[fieldState.error]} />
                )}
              </>
            )}
          />
        </div>
      </div>

      {/* address + number */}
      <div className="grid grid-cols-3 gap-3">
        <div className="col-span-2 flex flex-col gap-1 text-sm">
          <Controller
            name={`${namePrefix}address` as Path<T>}
            control={control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={`${idPrefix}address`}>Endereço</FieldLabel>
                <InputGroup className="bg-slate-200">
                  <InputGroupInput
                    {...field}
                    id={`${idPrefix}address`}
                    type="text"
                    aria-invalid={fieldState.invalid}
                    placeholder="Rua das Flores"
                    disabled={disabled}
                  />
                  <InputGroupAddon align="inline-end">
                    <Home className="text-foreground h-4 w-4" />
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
            name={`${namePrefix}address_number` as Path<T>}
            control={control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={`${idPrefix}address_number`}>
                  Número
                </FieldLabel>
                <InputGroup className="bg-slate-200">
                  <InputGroupInput
                    {...field}
                    id={`${idPrefix}address_number`}
                    type="text"
                    aria-invalid={fieldState.invalid}
                    placeholder="123"
                    disabled={disabled}
                  />
                </InputGroup>
                {fieldState.invalid && (
                  <FieldError errors={[fieldState.error]} />
                )}
              </>
            )}
          />
        </div>
      </div>

      {/* complement + neighborhood */}
      <div className="grid grid-cols-2 gap-3">
        <div className="flex flex-col gap-1 text-sm">
          <Controller
            name={`${namePrefix}complement` as Path<T>}
            control={control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={`${idPrefix}complement`}>
                  Complemento
                </FieldLabel>
                <InputGroup className="bg-slate-200">
                  <InputGroupInput
                    {...field}
                    id={`${idPrefix}complement`}
                    type="text"
                    aria-invalid={fieldState.invalid}
                    placeholder="Apto 12"
                    disabled={disabled}
                  />
                  <InputGroupAddon align="inline-end">
                    <FileText className="text-foreground h-4 w-4" />
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
            name={`${namePrefix}neighborhood` as Path<T>}
            control={control}
            render={({ field, fieldState }) => (
              <>
                <FieldLabel htmlFor={`${idPrefix}neighborhood`}>
                  Bairro
                </FieldLabel>
                <InputGroup className="bg-slate-200">
                  <InputGroupInput
                    {...field}
                    id={`${idPrefix}neighborhood`}
                    type="text"
                    aria-invalid={fieldState.invalid}
                    placeholder="Centro"
                    disabled={disabled}
                  />
                  <InputGroupAddon align="inline-end">
                    <Signpost className="text-foreground h-4 w-4" />
                  </InputGroupAddon>
                </InputGroup>
                {fieldState.invalid && (
                  <FieldError errors={[fieldState.error]} />
                )}
              </>
            )}
          />
        </div>
      </div>
    </>
  )
}
