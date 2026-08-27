// Labels for the promotion enrolment screens. Same shape as lib/account-category.ts:
// the enum values stay in English, mirroring the Go side, and everything an operator
// reads is Portuguese spelled out.

export const PROMO_STATUS_LABELS: Record<string, string> = {
  success: "Cadastrado",
  failure: "Recusado pelo site",
  unknown: "Sem resposta reconhecível",
  refused: "Não cadastrado",
  error: "Erro",
}

export const PROMO_STATUS_OPTIONS = [
  { value: "success", label: PROMO_STATUS_LABELS.success },
  { value: "failure", label: PROMO_STATUS_LABELS.failure },
  { value: "unknown", label: PROMO_STATUS_LABELS.unknown },
  { value: "refused", label: PROMO_STATUS_LABELS.refused },
  { value: "error", label: PROMO_STATUS_LABELS.error },
]

export function promoStatusLabel(status: string): string {
  return PROMO_STATUS_LABELS[status] ?? status
}

// Only a success is green. "unknown" is amber rather than red on purpose: the
// submission went through and nobody knows the outcome, which is a different problem
// from a refusal and calls for someone to look at the screenshot.
export function promoStatusTone(status: string): string {
  switch (status) {
    case "success":
      return "bg-emerald-100 text-emerald-800"
    case "unknown":
      return "bg-amber-100 text-amber-800"
    case "failure":
    case "error":
      return "bg-red-100 text-red-800"
    default:
      return "bg-slate-100 text-slate-700"
  }
}

// How the bot decided what to interact with, for the dry-run panel.
export function promoSourceLabel(source: string): string {
  switch (source) {
    case "recipe":
      return "pelos seletores da receita"
    case "mixed":
      return "parte pela receita, parte automaticamente"
    default:
      return "automaticamente"
  }
}

// A CPF is shown masked in listings. It is stored encrypted and the panel is allowed
// to display it - staff need the number - but a screen full of bare documents is not
// something to leave on a monitor.
export function maskCPF(cpf: string): string {
  const digits = cpf.replace(/\D/g, "")
  if (digits.length !== 11) return cpf
  return `${digits.slice(0, 3)}.***.***-${digits.slice(9)}`
}

export function formatCPF(cpf: string): string {
  const digits = cpf.replace(/\D/g, "")
  if (digits.length !== 11) return cpf
  return `${digits.slice(0, 3)}.${digits.slice(3, 6)}.${digits.slice(6, 9)}-${digits.slice(9)}`
}
