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

export const searchAirportsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(airportSchema).nullish(),
  }),
}
