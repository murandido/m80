"use server"

import { cookies } from "next/headers"
import { loginSchema } from "@/schemas/auth"
import { env } from "@/lib/env"
import { redirect } from "next/navigation"
import { z } from "zod"

// Reading the session lives in @/lib/session instead: this file is "use server",
// where every export has to be a plain async function, and those helpers are
// memoized with React's cache() so a render resolves the session once.

export async function loginAction(
  prevState: any,
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
      headers: { "Content-Type": "application/json" },
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

    // save in a secure cookie
    const cookieStore = await cookies()
    cookieStore.set("auth_token", token, {
      httpOnly: true,
      // TODO: activate this when launch
      // secure: env.NODE_ENV === "production",
      sameSite: "lax",
      maxAge: 60 * 60 * 24 * 7, // 7 days
    })
  } catch {
    return { error: "Connection error with the server." }
  }

  redirect("/dashboard")
}
