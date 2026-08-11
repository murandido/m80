import { toast } from "sonner"

// Writes to the clipboard and reports the outcome. Shared rather than local because the
// issuance copy menu and the keyboard shortcut that does the same thing live in different
// files, and a copy that silently failed is worse than one that says so.
export async function copyToClipboard(value: string, successMessage: string) {
  try {
    await navigator.clipboard.writeText(value)
    toast.success(successMessage)
  } catch {
    toast.error("Não foi possível copiar para a área de transferência.")
  }
}
