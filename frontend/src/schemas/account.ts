import { z } from "zod"

export const loyaltyProgramSummarySchema = z.object({
  id: z.string(),
  name: z.string(),
  code: z.string(),
  color: z.string(),
})

export type LoyaltyProgramSummary = z.infer<typeof loyaltyProgramSummarySchema>

export const accountNicknameSchema = z.object({
  id: z.string(),
  account_id: z.string(),
  loyalty_program_id: z.string(),
  nickname: z.string(),
  created_at: z.string(),
})

export type AccountNickname = z.infer<typeof accountNicknameSchema>

// The three categories, in the order the panel reads them. The values are the ones the
// API stores; the Portuguese labels live in account-category.ts, next to the badge.
export const accountCategorySchema = z.enum(["unidentified", "standard", "injunction"])

export type AccountCategory = z.infer<typeof accountCategorySchema>

// How much of the loyalty program's beneficiary cap the account has used. Null whenever
// there is no cap to report - an injunction suspends it, an unclassified account gives no
// basis to claim one, and a program nobody has researched has no limit. Null means "show
// no counter", which is not the same as a counter reading zero.
export const cpfUsageSchema = z.object({
  used: z.number(),
  limit: z.number(),
  remaining: z.number(),
  // "calendar_year" resets on 1 January; "rolling_12_months" slides day by day.
  window: z.enum(["calendar_year", "rolling_12_months"]),
  // "distinct_people" reuses a slot for someone already issued (Smiles, Azul);
  // "per_issuance" charges one for every issuance, same person or not (LATAM).
  mode: z.enum(["distinct_people", "per_issuance"]),
  resets_at: z.string().nullish(),
})

export type CPFUsage = z.infer<typeof cpfUsageSchema>

export const accountSchema = z.object({
  id: z.string(),
  loyalty_program_id: z.string(),
  loyalty_program: loyaltyProgramSummarySchema,
  login: z.string().nullable(),
  password: z.string().nullable(),
  holder_id: z.string().nullable(),
  administrator_id: z.string().nullable(),
  nicknames: z.array(accountNicknameSchema),
  // Defaulted rather than required: an older API that predates the category still answers
  // the panel, and the two are published by independent labels.
  category: accountCategorySchema.nullish().transform((value) => value ?? "unidentified"),
  // The account pays no redemption fee at all - on Azul that is the Diamante Unique tier.
  // Defaulted for the same reason as category: an API published before this field still
  // parses, and false is what every account meant before it existed.
  waives_redemption_fee: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  // Defaulted for the same reason: an account from before the points inventory existed
  // owns none, and a split of zero leaves the profit whole with whoever funded the batch.
  owns_point_inventory: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  default_profit_share_percent: z
    .number()
    .nullish()
    .transform((value) => value ?? 0),
  // The balance somebody last read off the airline's page, and the day they read it.
  // Null until somebody looks - no statement export carries a position, so this is the
  // only place the real number can come from.
  real_balance_points: z.number().nullish(),
  real_balance_checked_on: z.string().nullish(),
  // What the costing says the account still has: batches, less what was issued. Null on
  // an account that does not own its point inventory, where there is no credit side to
  // derive it from.
  expected_balance_points: z.number().nullish(),
  // Whether the Azul statement sync can read this account (a customerKey was discovered).
  // Defaulted like the flags above: an API published before this field still parses, and
  // false is what every account meant before it existed.
  azul_connected: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  // When the statement sync last succeeded, for the card's "atualizado há…". Null until
  // the first successful sync.
  azul_last_synced_at: z.string().nullish(),
  cpf_usage: cpfUsageSchema.nullish(),
  complete: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
  // Set only on an archived account, which the account list never returns - it only
  // ever arrives embedded in a past issuance, so an old row can still be labelled
  // with it while the pickers leave it out.
  deleted_at: z.string().nullish(),
})

export type Account = z.infer<typeof accountSchema>

// What a picker reads off an account, and nothing else. Accounts have no name of their
// own, so identifying one takes the holder, the login and the aliases; the program comes
// as an id the caller resolves against the program list it already has, rather than as a
// copy of the program embedded once per account.
//
// The management screen still reads the full record - see accountSchema. This exists so
// that reaching for a field a picker does not carry is a compile error rather than a
// silent 300 KB.
export const accountNicknameOptionSchema = z.object({
  id: z.string(),
  nickname: z.string(),
})

export const accountOptionSchema = z.object({
  id: z.string(),
  loyalty_program_id: z.string(),
  login: z.string().nullable(),
  holder_id: z.string().nullable(),
  nicknames: z.array(accountNicknameOptionSchema),
  // Both are omitted by the API whenever they carry no information - false and null -
  // which on an unpaginated feed is most rows. Defaulted here for the same reason
  // accountSchema defaults them: absent and false have always meant the same thing.
  waives_redemption_fee: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  // Absent means false, the same contract as the flag above: the API omits it on every
  // ordinary account, which is nearly all of them.
  owns_point_inventory: z
    .boolean()
    .nullish()
    .transform((value) => value ?? false),
  deleted_at: z.string().nullish(),
})

export type AccountOption = z.infer<typeof accountOptionSchema>

export const getAccountOptionsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(accountOptionSchema).nullish(),
  }),
}

// GET /accounts/{id}, read for one field: administrator_id, which the pickers do not carry.
// Deliberately narrow instead of reusing accountSchema - this parses a record the caller only
// wants to ask a question about, so requiring all sixteen fields to line up would turn an
// unrelated API change into a failed link.
export const getAccountAdministratorSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        id: z.string(),
        administrator_id: z.string().nullish(),
      })
      .nullish(),
  }),
}

export const getAccountsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(accountSchema).nullish(),
  }),
}

// GET /accounts/{id}: the full account, read whole for the per-account statement page header.
export const getAccountSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountSchema.nullish(),
  }),
}

// GET /accounts/manage. The plain /accounts stays an array because the pickers on the
// issuances grid, the sales screens and the importer read it whole.
export const listAccountsSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        accounts: z.array(accountSchema).nullish(),
        total: z.number(),
        page: z.number(),
        limit: z.number(),
      })
      .nullish(),
  }),
}

export type AccountPage = NonNullable<z.infer<typeof listAccountsSchema.response>["data"]>

const optionalTextField = z.string().optional().or(z.literal(""))
const optionalUuidField = z.union([z.literal(""), z.string().uuid("UUID inválido")])

export const createAccountSchema = {
  request: z.object({
    loyalty_program_id: z.string().uuid("UUID inválido"),
    login: optionalTextField,
    password: optionalTextField,
    holder_id: optionalUuidField,
    administrator_id: optionalUuidField,
    category: accountCategorySchema.optional(),
    waives_redemption_fee: z.boolean().optional(),
    owns_point_inventory: z.boolean().optional(),
    default_profit_share_percent: z.number().min(0).max(100).optional(),
    nicknames: z.array(z.string().min(1)).optional(),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountSchema.nullish(),
  }),
}

export const updateAccountSchema = {
  request: z.object({
    loyalty_program_id: optionalUuidField,
    login: optionalTextField,
    password: optionalTextField,
    holder_id: optionalUuidField,
    administrator_id: optionalUuidField,
    category: accountCategorySchema.optional(),
    waives_redemption_fee: z.boolean().optional(),
    owns_point_inventory: z.boolean().optional(),
    default_profit_share_percent: z.number().min(0).max(100).optional(),
    // The real balance is no longer editable here: the Azul statement sync owns it.
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountSchema.nullish(),
  }),
}

export const deleteAccountSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}

export const createAccountNicknameSchema = {
  request: z.object({
    loyalty_program_id: z.string().uuid("UUID inválido"),
    nickname: z.string().min(1, "Apelido é obrigatório"),
  }),
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountNicknameSchema.nullish(),
  }),
}

export const deleteAccountNicknameSchema = {
  response: z.object({
    message: z.string(),
    data: z.null().nullish(),
  }),
}

export const checkAccountNicknameAvailabilitySchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.object({ available: z.boolean() }).nullish(),
  }),
}

// Same idea as mergePersonPreviewSchema: the counts the panel shows before an
// irreversible merge. issuances includes the ones in the grid's trash.
export const mergeAccountPreviewSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z
      .object({
        issuances: z.number(),
        nicknames: z.number(),
        fields_to_fill: z.array(z.string()).nullish(),
        colliding_nicknames: z.array(z.string()).nullish(),
      })
      .nullish(),
  }),
}

export const mergeAccountSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: accountSchema.nullish(),
  }),
}

export type MergeAccountPreview = NonNullable<
  z.infer<typeof mergeAccountPreviewSchema.response>["data"]
>

// One person on an account's beneficiary ledger. The CPF is never returned in full because
// it is never stored in full: cpf_masked is built from the number at write time, and
// has_cpf says whether the row is identified by a document at all - which is what tells the
// operator how much to trust the match.
export const accountBeneficiarySchema = z.object({
  id: z.string(),
  account_id: z.string(),
  name: z.string(),
  cpf_masked: z.string().nullish(),
  has_cpf: z.boolean(),
  is_holder: z.boolean(),
  uses: z.number(),
  last_used_at: z.string().nullish(),
  created_at: z.string(),
})

export type AccountBeneficiary = z.infer<typeof accountBeneficiarySchema>

export const getAccountBeneficiariesSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.array(accountBeneficiarySchema).nullish(),
  }),
}

export const accountBeneficiaryMutationSchema = {
  response: z.object({
    success: z.boolean(),
    message: z.string(),
    data: z.unknown().nullish(),
  }),
}
