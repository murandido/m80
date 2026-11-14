import { z } from "zod"

// A recipe describes how one site's enrolment form is driven. Every selector is
// optional: blank means "let the bot work this piece out on its own", which is why the
// form starts empty and a conventional site never needs anything typed into it.
export const promoRecipeSchema = z.object({
  id: z.string(),
  domain: z.string(),
  name: z.string(),
  loyalty_program_id: z.string(),
  loyalty_program_name: z.string().nullish(),
  cpf_selector: z.string().nullish(),
  checkbox_selector: z.string().nullish(),
  submit_selector: z.string().nullish(),
  success_pattern: z.string().nullish(),
  failure_pattern: z.string().nullish(),
  wait_seconds: z.number(),
  active: z.boolean(),
  notes: z.string().nullish(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type PromoRecipe = z.infer<typeof promoRecipeSchema>

// wait_seconds is a string in the form because that is what a text input holds.
const waitSecondsField = z
  .string()
  .refine(
    (value) =>
      value.trim() === "" ||
      (/^\d+$/.test(value.trim()) && Number(value.trim()) <= 120),
    { message: "Use um número de segundos entre 0 e 120" },
  )

const recipeFields = {
  name: z.string().min(1, "Nome é obrigatório"),
  domain: z.string().min(1, "Domínio é obrigatório"),
  loyalty_program_id: z.string().min(1, "Programa de fidelidade é obrigatório"),
  cpf_selector: z.string(),
  checkbox_selector: z.string(),
  submit_selector: z.string(),
  success_pattern: z.string(),
  failure_pattern: z.string(),
  wait_seconds: waitSecondsField,
  active: z.boolean(),
  notes: z.string(),
}

type RecipeFormFields = {
  cpf_selector: string
  checkbox_selector: string
  submit_selector: string
  success_pattern: string
  failure_pattern: string
  wait_seconds: string
  notes: string
}

// Turns the form's strings into what the Go API expects. A blank field becomes null,
// which the backend reads as "clear it" - so emptying a selector is how one pinned by
// mistake is handed back to the heuristic.
export function toPromoRecipePayload<T extends RecipeFormFields>(data: T) {
  const blankToNull = (value: string) =>
    value.trim() === "" ? null : value.trim()

  return {
    ...data,
    cpf_selector: blankToNull(data.cpf_selector),
    checkbox_selector: blankToNull(data.checkbox_selector),
    submit_selector: blankToNull(data.submit_selector),
    success_pattern: blankToNull(data.success_pattern),
    failure_pattern: blankToNull(data.failure_pattern),
    notes: blankToNull(data.notes),
    wait_seconds:
      data.wait_seconds.trim() === "" ? 5 : Number(data.wait_seconds.trim()),
  }
}

export const getPromoRecipesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(promoRecipeSchema).nullish(),
  }),
}

const recipeSingleResponse = z.object({
  success: z.boolean(),
  message: z.string(),
  data: promoRecipeSchema.nullish(),
})

export const createPromoRecipeSchema = {
  request: z.object(recipeFields),
  response: recipeSingleResponse,
}

export const updatePromoRecipeSchema = {
  request: z.object(recipeFields),
  response: recipeSingleResponse,
}

// The dry run. Resolved is null when the page offered nothing that looks like an
// enrolment form; the screenshot comes back either way, and is the point of the whole
// exercise - the author sees what the bot would see.
export const testPromoRecipeSchema = {
  request: z.object({
    url: z.url("Informe o link completo da promoção"),
    cpf_selector: z.string(),
    checkbox_selector: z.string(),
    submit_selector: z.string(),
    success_pattern: z.string(),
    failure_pattern: z.string(),
    wait_seconds: waitSecondsField,
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        resolved: z
          .object({
            cpf_field: z.string(),
            checkbox: z.boolean(),
            submit: z.string(),
            source: z.string(),
          })
          .nullish(),
        error: z.string().nullish(),
        screenshot_base64: z.string().nullish(),
      })
      .nullish(),
  }),
}

export const loyaltyDocumentSchema = z.object({
  id: z.string(),
  name: z.string(),
  cpf: z.string(),
  loyalty_program_id: z.string(),
  loyalty_program_name: z.string().nullish(),
  active: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
})

export type LoyaltyDocument = z.infer<typeof loyaltyDocumentSchema>

const documentFields = {
  name: z.string().min(1, "Nome é obrigatório"),
  // Only the digit count is checked here. The check digits are verified by the API,
  // which is the one place that can reject a plausible-looking but invalid number
  // before it is ever submitted to a promotion.
  cpf: z
    .string()
    .refine((value) => value.replace(/\D/g, "").length === 11, {
      message: "O CPF precisa ter 11 dígitos",
    }),
  loyalty_program_id: z.string().min(1, "Programa de fidelidade é obrigatório"),
  active: z.boolean(),
}

// GET /loyalty-documents/manage, for the screen. The plain route stays an array.
export const listLoyaltyDocumentsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        documents: z.array(loyaltyDocumentSchema).nullish(),
        total: z.number(),
        page: z.number(),
        limit: z.number(),
      })
      .nullish(),
  }),
}

export type LoyaltyDocumentPage = NonNullable<
  z.infer<typeof listLoyaltyDocumentsSchema.response>["data"]
>

const documentSingleResponse = z.object({
  success: z.boolean(),
  message: z.string(),
  data: loyaltyDocumentSchema.nullish(),
})

export const createLoyaltyDocumentSchema = {
  request: z.object(documentFields),
  response: documentSingleResponse,
}

export const updateLoyaltyDocumentSchema = {
  request: z.object(documentFields),
  response: documentSingleResponse,
}

export const promoEnrollmentSchema = z.object({
  id: z.string(),
  url: z.string(),
  url_key: z.string(),
  document_id: z.string(),
  document_name: z.string().nullish(),
  recipe_id: z.string().nullish(),
  recipe_name: z.string().nullish(),
  status: z.string(),
  message: z.string().nullish(),
  created_by: z.string().nullish(),
  chat_jid: z.string().nullish(),
  created_at: z.string(),
})

export type PromoEnrollment = z.infer<typeof promoEnrollmentSchema>

export const getPromoEnrollmentsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        enrollments: z.array(promoEnrollmentSchema).nullish(),
        total: z.number(),
        page: z.number(),
        limit: z.number(),
      })
      .nullish(),
  }),
}
