import { z } from "zod"
import { issuanceFlightSchema } from "@/schemas/issuance"

// The seats and the baggage of an issuance - the two things the airline changes
// after issuing, and the two the operator can now correct from the ticket page.
//
// Each is symmetric on the wire: the shape the GET returns is the shape the PUT
// accepts, so the panel edits the object it was handed instead of translating
// between a read model and a write model that can drift apart.

export const issuanceSeatSchema = z.object({
  passenger_id: z.string(),
  flight_id: z.string(),
  // null is a real state, not a missing value: several airlines only assign a
  // seat at check-in.
  seat: z.string().nullable(),
})

export type IssuanceSeat = z.infer<typeof issuanceSeatSchema>

export const issuanceBaggageSchema = z.object({
  passenger_id: z.string(),
  direction: z.enum(["outbound", "inbound"]),
  // The entitlement's place in the airline's own list, which is what decides
  // the icon on the ticket - the first one is the cabin bag. Sent back
  // untouched rather than renumbered.
  position: z.number(),
  // null when the airline reports a bare count with no weight, as GOL does.
  weight: z.string().nullable(),
  quantity: z.number(),
})

export type IssuanceBaggage = z.infer<typeof issuanceBaggageSchema>

export const issuancePassengerSchema = z.object({
  id: z.string(),
  issuance_id: z.string(),
  first_name: z.string(),
  last_name: z.string(),
  type: z.string(),
  position: z.number(),
})

export type IssuancePassenger = z.infer<typeof issuancePassengerSchema>

const envelope = <T extends z.ZodTypeAny>(data: T) =>
  z.object({
    success: z.boolean(),
    message: z.string(),
    data: data.nullish(),
  })

export const getIssuanceSeatsSchema = {
  response: envelope(z.array(issuanceSeatSchema)),
}

export const putIssuanceSeatsSchema = {
  request: z.object({ seats: z.array(issuanceSeatSchema) }),
  response: envelope(z.array(issuanceSeatSchema)),
}

export const getIssuanceBaggageSchema = {
  response: envelope(z.array(issuanceBaggageSchema)),
}

export const putIssuanceBaggageSchema = {
  request: z.object({ baggage: z.array(issuanceBaggageSchema) }),
  response: envelope(z.array(issuanceBaggageSchema)),
}

export const getIssuancePassengersSchema = {
  response: envelope(z.array(issuancePassengerSchema)),
}

// Only the legs are picked out of the issuance here. The editor needs them to
// label a seat field ("Ida · T1 · GRU–GIG") and nothing else on the issuance
// matters to it, so parsing the whole record would tie this panel to every
// future field of an unrelated resource.
export const getIssuanceLegsSchema = {
  response: envelope(
    z.object({
      flights: z.array(issuanceFlightSchema).nullish(),
    }),
  ),
}
