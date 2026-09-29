import { redirect } from "next/navigation"

// The owners screen folded into the accounts screen, where a holder and their loyalty
// accounts are one row. Kept as a redirect for bookmarks and tabs left open on it.
export default function PeoplePage() {
  redirect("/dashboard/accounts")
}
