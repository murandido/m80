import { z } from "zod"

export const airlineSchema = z.object({
  id: z.string(),
  name: z.string(),
  code: z.string(),
  traveos_supported: z.boolean(),
  requires_last_name: z.boolean(),
  requires_purchase_number: z.boolean(),
  traveos_airline_id: z.number().nullish(),
  brand_color: z.string().nullish(),
  created_at: z.string(),
  updated_at: z.string(),
})

// The two Traveos fields are kept as strings in the form because that is what a
// text input holds, and are converted on the way out by toAirlineRequestPayload.
// Validating here rather than only server-side means the operator sees the
// mistake next to the field instead of as a toast.
const traveosAirlineIdField = z
  .string()
  .refine((value) => value === "" || /^\d+$/.test(value.trim()), {
    message: "Use só números (é o id da cia no catálogo da Traveos)",
  })

const brandColorField = z
  .string()
  .refine((value) => value === "" || /^#[0-9a-fA-F]{6}$/.test(value.trim()), {
    message: "Use o formato #RRGGBB",
  })

type AirlineFormFields = {
  traveos_airline_id: string
  brand_color: string
}

// Turns the form's strings into what the Go API expects. An empty field becomes
// null, which on these two fields the backend reads as "clear it" - so emptying
// the input is how a Traveos id typed by mistake is taken back.
export function toAirlineRequestPayload<T extends AirlineFormFields>(data: T) {
  return {
    ...data,
    traveos_airline_id:
      data.traveos_airline_id.trim() === "" ? null : Number(data.traveos_airline_id.trim()),
    brand_color:
      data.brand_color.trim() === "" ? null : data.brand_color.trim().toUpperCase(),
  }
}

export type Airline = z.infer<typeof airlineSchema>

export const getAirlinesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(airlineSchema).nullish(),
  }),
}

export const createAirlineSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    code: z.string().min(1, "Código é obrigatório"),
    traveos_supported: z.boolean(),
    requires_last_name: z.boolean(),
    requires_purchase_number: z.boolean(),
    traveos_airline_id: traveosAirlineIdField,
    brand_color: brandColorField,
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: airlineSchema.nullish(),
  }),
}

export const updateAirlineSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    code: z.string().min(1, "Código é obrigatório"),
    traveos_supported: z.boolean(),
    requires_last_name: z.boolean(),
    requires_purchase_number: z.boolean(),
    traveos_airline_id: traveosAirlineIdField,
    brand_color: brandColorField,
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: airlineSchema.nullish(),
  }),
}

export const deleteAirlineSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}
