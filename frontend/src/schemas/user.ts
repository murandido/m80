import { z } from "zod"

// The user_role Postgres enum, in one place. Every schema below reuses it: if any single
// one of them did not know a role the backend can return, safeParse fails and the whole
// screen (or the session) breaks, not just that field.
export const userRoleSchema = z.enum(["user", "admin", "dev"])

// What each role is called on screen. Kept here rather than in a component so server
// and client code can both read it - a "use client" module hands a server component
// a reference, not the object.
export const userRoleLabels: Record<z.infer<typeof userRoleSchema>, string> = {
  admin: "Administrador",
  user: "Usuário",
  dev: "Desenvolvedor",
}

export const userSchema = z.object({
  id: z.string(),
  name: z.string(),
  phone: z.string(),
  role: userRoleSchema,
  // The agencies the user may see. Always empty for a dev, whose role covers every agency.
  // Defaulted so a response from an API that predates the field still parses - and null
  // too, not just absent: a Go nil slice serializes as null, and one embedded user sending
  // it failed the whole issuance list.
  agency_ids: z
    .array(z.string())
    .nullish()
    .transform((value) => value ?? []),
  created_at: z.string(),
})

export const getUsersSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(userSchema).nullish(),
  }),
}

export type User = z.infer<typeof userSchema>

export const getMeSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: userSchema.nullish(),
  }),
}

export const updateUserSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    phone: z
      .string()
      .min(10, "Telefone deve ter no mínimo 10 dígitos")
      .max(11, "Telefone deve ter no máximo 11 dígitos")
      .regex(/^\d+$/, "Apenas números são permitidos"),
    password: z.union([
      z.literal(""),
      z
        .string()
        .min(6, "A senha precisa ter no mínimo 6 caracteres")
        .max(20, "A senha pode ter no máximo 20 caracteres"),
    ]),
    role: userRoleSchema,
    // Only a dev may send it - the API refuses it from anyone else - so it stays undefined,
    // and out of the request, when an admin edits.
    agency_ids: z.array(z.string()).optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        id: z.string(),
        name: z.string(),
        phone: z.string(),
        role: userRoleSchema,
        created_at: z.string(),
      })
      .nullish(),
  }),
}

export const createUserSchema = {
  request: z.object({
    name: z.string().min(1, "Nome é obrigatório"),
    phone: z
      .string()
      .min(10, "Telefone deve ter no mínimo 10 dígitos")
      .max(11, "Telefone deve ter no máximo 11 dígitos")
      .regex(/^\d+$/, "Apenas números são permitidos"),
    password: z
      .string()
      .min(6, "A senha precisa ter no mínimo 6 caracteres")
      .max(20, "A senha pode ter no máximo 20 caracteres"),
    role: userRoleSchema,
    // See updateUserSchema: only a dev sends it.
    agency_ids: z.array(z.string()).optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        id: z.string(),
        name: z.string(),
        phone: z.string(),
        role: userRoleSchema,
        created_at: z.string(),
      })
      .nullish(),
  }),
}

// updateMeSchemaFor builds the self-service profile schema around the phone the user
// has now: changing it, or the password, asks for the current password - the backend
// enforces the same rule, this only says so before the round trip.
export function updateMeSchemaFor(originalPhone: string) {
  return z
    .object({
      name: z.string().min(1, "Nome é obrigatório"),
      phone: z
        .string()
        .min(10, "Telefone deve ter no mínimo 10 dígitos")
        .max(11, "Telefone deve ter no máximo 11 dígitos")
        .regex(/^\d+$/, "Apenas números são permitidos"),
      new_password: z.union([
        z.literal(""),
        z
          .string()
          .min(6, "A senha precisa ter no mínimo 6 caracteres")
          .max(20, "A senha pode ter no máximo 20 caracteres"),
      ]),
      current_password: z.string(),
    })
    .superRefine((data, ctx) => {
      const touchesLogin =
        data.new_password !== "" || data.phone !== originalPhone
      if (touchesLogin && data.current_password === "") {
        ctx.addIssue({
          code: "custom",
          path: ["current_password"],
          message: "Informe a senha atual para trocar a senha ou o telefone",
        })
      }
    })
}

export type UpdateMeRequest = z.infer<ReturnType<typeof updateMeSchemaFor>>
