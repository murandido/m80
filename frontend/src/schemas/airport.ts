import { z } from "zod"

export const airportSchema = z.object({
  id: z.string(),
  iata_code: z.string(),
  name: z.string(),
  city: z.string().nullable(),
  state: z.string().nullable(),
  country: z.string(),
})

export type Airport = z.infer<typeof airportSchema>

export const getAirportsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(airportSchema).nullish(),
  }),
}

export const listAirportsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        airports: z.array(airportSchema).nullish(),
        total: z.number(),
        page: z.number(),
        limit: z.number(),
      })
      .nullish(),
  }),
}

const airportRequestFields = {
  iata_code: z.string().min(1, "Código IATA é obrigatório"),
  name: z.string().min(1, "Nome é obrigatório"),
  city: z.string(),
  state: z.string(),
  country: z.string().min(1, "País é obrigatório"),
}

export const createAirportSchema = {
  request: z.object(airportRequestFields),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: airportSchema.nullish(),
  }),
}

export const updateAirportSchema = {
  request: z.object(airportRequestFields),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: airportSchema.nullish(),
  }),
}
