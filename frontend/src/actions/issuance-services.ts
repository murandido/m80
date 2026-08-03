"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { IssuanceFlight } from "@/schemas/issuance"
import {
  getIssuanceBaggageSchema,
  getIssuanceLegsSchema,
  getIssuancePassengersSchema,
  getIssuanceSeatsSchema,
  IssuanceBaggage,
  IssuancePassenger,
  IssuanceSeat,
  putIssuanceBaggageSchema,
  putIssuanceSeatsSchema,
} from "@/schemas/issuance-services"

// The seats and the baggage of an issuance. The reads are here rather than
// inline in the page because the writes need them too - the panel refetches
// after a save - and one place that knows the URL is one place to fix it.

async function authHeaders(): Promise<Record<string, string>> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  return { Authorization: `Bearer ${token}` }
}

export async function getIssuanceSeatsAction(
  issuanceID: string,
): Promise<{ error: string } | { seats: IssuanceSeat[] }> {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceID}/seats`, {
      headers: await authHeaders(),
      cache: "no-store",
    })
    const parsed = getIssuanceSeatsSchema.response.safeParse(await res.json())

    if (!res.ok || !parsed.data) {
      return { error: parsed.data?.message || "Erro ao carregar os assentos." }
    }
    return { seats: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function getIssuanceBaggageAction(
  issuanceID: string,
): Promise<{ error: string } | { baggage: IssuanceBaggage[] }> {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceID}/baggage`, {
      headers: await authHeaders(),
      cache: "no-store",
    })
    const parsed = getIssuanceBaggageSchema.response.safeParse(await res.json())

    if (!res.ok || !parsed.data) {
      return { error: parsed.data?.message || "Erro ao carregar as bagagens." }
    }
    return { baggage: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function getIssuancePassengersAction(
  issuanceID: string,
): Promise<{ error: string } | { passengers: IssuancePassenger[] }> {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceID}/passengers`, {
      headers: await authHeaders(),
      cache: "no-store",
    })
    const parsed = getIssuancePassengersSchema.response.safeParse(await res.json())

    if (!res.ok || !parsed.data) {
      return { error: parsed.data?.message || "Erro ao carregar os passageiros." }
    }
    return { passengers: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function getIssuanceLegsAction(
  issuanceID: string,
): Promise<{ error: string } | { flights: IssuanceFlight[] }> {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceID}`, {
      headers: await authHeaders(),
      cache: "no-store",
    })
    const parsed = getIssuanceLegsSchema.response.safeParse(await res.json())

    if (!res.ok || !parsed.data) {
      return { error: parsed.data?.message || "Erro ao carregar os trechos." }
    }
    return { flights: parsed.data.data?.flights ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Both writes send the complete set, never a delta. The panel may have been
// open while the bot re-synced the same issuance from Traveos, and the server
// replaces rather than merges for exactly that reason.

export async function putIssuanceSeatsAction(
  issuanceID: string,
  seats: IssuanceSeat[],
): Promise<{ error: string } | { seats: IssuanceSeat[] }> {
  const body = putIssuanceSeatsSchema.request.safeParse({ seats })
  if (!body.success) {
    return { error: "Assentos inválidos." }
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceID}/seats`, {
      method: "PUT",
      headers: { ...(await authHeaders()), "Content-Type": "application/json" },
      body: JSON.stringify(body.data),
      cache: "no-store",
    })
    const parsed = putIssuanceSeatsSchema.response.safeParse(await res.json())

    if (!res.ok || !parsed.data) {
      return { error: parsed.data?.message || "Erro ao salvar os assentos." }
    }
    return { seats: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function putIssuanceBaggageAction(
  issuanceID: string,
  baggage: IssuanceBaggage[],
): Promise<{ error: string } | { baggage: IssuanceBaggage[] }> {
  const body = putIssuanceBaggageSchema.request.safeParse({ baggage })
  if (!body.success) {
    return { error: "Bagagens inválidas." }
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${issuanceID}/baggage`, {
      method: "PUT",
      headers: { ...(await authHeaders()), "Content-Type": "application/json" },
      body: JSON.stringify(body.data),
      cache: "no-store",
    })
    const parsed = putIssuanceBaggageSchema.response.safeParse(await res.json())

    if (!res.ok || !parsed.data) {
      return { error: parsed.data?.message || "Erro ao salvar as bagagens." }
    }
    return { baggage: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}
