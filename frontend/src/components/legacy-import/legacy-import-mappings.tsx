"use client"

import { AccountCombobox } from "@/components/accounts/account-combobox"
import { AgencyCombobox } from "@/components/agencies/agency-combobox"
import { UserCombobox } from "@/components/users/user-combobox"
import type { Account } from "@/schemas/account"
import type { Agency } from "@/schemas/agency"
import type { Person } from "@/schemas/person"
import type { User } from "@/schemas/user"
import type {
  LegacyImportPendingAccount,
  LegacyImportPendingIssuer,
} from "@/schemas/legacy-import"

interface LegacyImportMappingsProps {
  pendingAccounts: LegacyImportPendingAccount[]
  pendingIssuers: LegacyImportPendingIssuer[]
  accounts: Account[]
  people: Person[]
  users: User[]
  agencies: Agency[]

  accountChoices: Record<string, string>
  issuerChoices: Record<string, string>
  agencyId: string

  onAccountChoice: (
    programCode: string,
    label: string,
    accountId: string,
  ) => void
  onIssuerChoice: (issuerName: string, userId: string) => void
  onAgencyChoice: (agencyId: string) => void
  onCreateAccount: (pending: LegacyImportPendingAccount, name: string) => void
  disabled?: boolean
}

// The key an account mapping is stored under: the alias alone is not enough, because
// account_nicknames is unique per loyalty program - "WESLEY" on Azul and "WESLEY" on
// Smiles are two different accounts. Joined on a control character neither half can
// contain, so an alias with spaces in it ("CONTAS MARCELO") still splits back cleanly.
const ACCOUNT_CHOICE_SEPARATOR = "\u001f"

export function accountChoiceKey(programCode: string, label: string): string {
  return `${programCode}${ACCOUNT_CHOICE_SEPARATOR}${label}`
}

export function splitAccountChoiceKey(key: string): [string, string] {
  const [programCode, label] = key.split(ACCOUNT_CHOICE_SEPARATOR)
  return [programCode, label ?? ""]
}

// The two questions the spreadsheet cannot answer for itself, plus the one it never
// asked. Everything the file *can* say for itself is already resolved by the time this
// renders - these are only the leftovers.
export function LegacyImportMappings({
  pendingAccounts,
  pendingIssuers,
  accounts,
  people,
  users,
  agencies,
  accountChoices,
  issuerChoices,
  agencyId,
  onAccountChoice,
  onIssuerChoice,
  onAgencyChoice,
  onCreateAccount,
  disabled,
}: LegacyImportMappingsProps) {
  return (
    <div className="space-y-6">
      <section className="space-y-2">
        <h2 className="text-sm font-medium text-slate-800">
          Agência padrão do dia
        </h2>
        <p className="text-muted-foreground text-xs">
          A planilha não tem coluna de agência. Esta é a agência que cada linha
          usa até você trocá-la na coluna Agência da prévia, linha a linha.
        </p>
        <div className="max-w-sm">
          <AgencyCombobox
            agencies={agencies}
            value={agencyId}
            onChange={onAgencyChoice}
            clearable={false}
            disabled={disabled}
          />
        </div>
      </section>

      {pendingIssuers.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-sm font-medium text-slate-800">
            Emissores ({pendingIssuers.length})
          </h2>
          <p className="text-muted-foreground text-xs">
            O nome vem do título do bloco na planilha. Um bloco pode citar duas
            pessoas — escolha quem fica com as emissões dele.
          </p>
          <div className="space-y-2">
            {pendingIssuers.map((pending) => (
              <div
                key={pending.issuer_name}
                className="flex flex-wrap items-center gap-3 rounded-md border p-3"
              >
                <span className="min-w-40 text-sm font-medium">
                  {pending.issuer_name}
                </span>
                <span className="text-muted-foreground text-xs">
                  {pending.row_count} linha{pending.row_count === 1 ? "" : "s"}
                </span>
                <div className="ml-auto w-64">
                  <UserCombobox
                    users={users}
                    // The server's suggestion is a first-name match and nothing more,
                    // so it seeds the control and waits to be confirmed.
                    value={
                      issuerChoices[pending.issuer_name] ??
                      pending.suggested_user_id
                    }
                    onChange={(userId) =>
                      onIssuerChoice(pending.issuer_name, userId)
                    }
                    placeholder="Escolher usuário..."
                    disabled={disabled}
                  />
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {pendingAccounts.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-sm font-medium text-slate-800">
            Contas ({pendingAccounts.length})
          </h2>
          <p className="text-muted-foreground text-xs">
            Apelidos que a planilha usa e o sistema ainda não conhece. Ao
            importar, o apelido é registrado na conta escolhida — então nos
            próximos dias ele já vem resolvido, e o bot passa a entendê-lo
            também.
          </p>
          <div className="space-y-2">
            {pendingAccounts.map((pending) => {
              const key = accountChoiceKey(
                pending.program_code,
                pending.account_label,
              )
              return (
                <div
                  key={key}
                  className="flex flex-wrap items-center gap-3 rounded-md border p-3"
                >
                  <span className="min-w-40 text-sm font-medium">
                    {pending.account_label}
                  </span>
                  <span className="text-muted-foreground text-xs">
                    {pending.program_name} · {pending.row_count} linha
                    {pending.row_count === 1 ? "" : "s"}
                  </span>
                  <div className="ml-auto w-72">
                    <AccountCombobox
                      // Only accounts of the same loyalty program: an alias belongs to
                      // one program, and offering the others would invite a mapping
                      // the database would refuse.
                      accounts={accounts.filter(
                        (account) =>
                          account.loyalty_program_id ===
                          pending.loyalty_program_id,
                      )}
                      people={people}
                      value={accountChoices[key] ?? ""}
                      onChange={(accountId) =>
                        onAccountChoice(
                          pending.program_code,
                          pending.account_label,
                          accountId,
                        )
                      }
                      // Same entry the issuances grid offers in its Conta column: when
                      // the alias belongs to nobody yet, registering it here beats
                      // leaving the screen to do it in Contas and coming back.
                      onCreate={(name) => onCreateAccount(pending, name)}
                      // The alias the spreadsheet uses: the search opens already
                      // filtered by it, and "Criar conta" already names it.
                      initialQuery={pending.account_label}
                      placeholder="Escolher conta..."
                      disabled={disabled}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </section>
      )}
    </div>
  )
}
