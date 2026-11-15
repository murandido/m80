"use server"

import { cookies } from "next/headers"
import { loginSchema } from "@/schemas/auth"
import { env } from "@/lib/env"
import { clientIpHeader } from "@/lib/client-ip"
import { SESSION_COOKIE, sessionCookieOptions } from "@/lib/session-cookie"
import { redirect } from "next/navigation"
import { z } from "zod"

// Reading the session lives in @/lib/session instead: this file is "use server",
// where every export has to be a plain async function, and those helpers are
// memoized with React's cache() so a render resolves the session once.

export async function loginAction(
  _prevState: unknown,
  data: z.infer<typeof loginSchema.request>,
) {
  const validateFields = loginSchema.request.safeParse(data)

  if (!validateFields.success) {
    return {
      error: validateFields.error.message,
    }
  }

  const { phone, password } = validateFields.data

  try {
    const baseUrl = env.BACKEND_URL
    const res = await fetch(`${baseUrl}/api/v1/auth/login`, {
      method: "POST",
      // Without this the API sees this container as the caller and its per-IP
      // login limit becomes one budget for everybody - see clientIpHeader.
      headers: { "Content-Type": "application/json", ...(await clientIpHeader()) },
      body: JSON.stringify({ phone, password }),
    })

    const json = await res.json()

    const responseBody = loginSchema.response.safeParse(json)

    if (
      !res.ok ||
      !json.success ||
      !responseBody.success ||
      !responseBody.data.data
    ) {
      return { error: responseBody.data?.message || "Login failed." }
    }

    const { token } = responseBody.data.data

    // Same name and attributes the proxy uses when it renews the session.
    const cookieStore = await cookies()
    cookieStore.set(SESSION_COOKIE, token, sessionCookieOptions)
  } catch {
    return { error: "Connection error with the server." }
  }

  redirect("/dashboard")
}
