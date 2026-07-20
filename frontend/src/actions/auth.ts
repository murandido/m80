"use server"

import { cookies } from "next/headers"
import { loginSchema } from "@/schemas/auth"
import { getMeSchema, User } from "@/schemas/user"
import { env } from "@/lib/env"
import { redirect } from "next/navigation"
import { z } from "zod"

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

export async function verifySession() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  if (!token) {
    redirect("/")
  }

  let isValid = false

  try {
    const baseUrl = env.BACKEND_URL
    const res = await fetch(`${baseUrl}/api/v1/auth/me`, {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    })

    isValid = res.ok
  } catch {
    isValid = false
  }

  if (!isValid) {
    redirect("/api/logout")
  }

  return true
}

// getSessionUser returns the authenticated user from GET /auth/me, or null when
// there is no valid session. Used to make role-aware UI decisions (e.g. hiding
// admin-only navigation entries).
export async function getSessionUser(): Promise<User | null> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  if (!token) {
    return null
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/auth/me`, {
      headers: { Authorization: `Bearer ${token}` },
    })

    const json = await res.json()
    const parsed = getMeSchema.response.safeParse(json)

    if (!res.ok || !parsed.success || !parsed.data.data) {
      return null
    }

    return parsed.data.data
  } catch {
    return null
  }
}

// requireAdmin guards admin-only pages: non-admins are redirected back to the
// dashboard. Backend routes still enforce admin independently — this is the UI
// layer of the same rule.
export async function requireAdmin(): Promise<User> {
  const user = await getSessionUser()

  if (!user || user.role !== "admin") {
    redirect("/dashboard")
  }

  return user
}
