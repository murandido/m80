import type { Agency } from "@/schemas/agency"

// The agency a screen lands on when none was picked - the same answer the API gives
// (access.ResolveDefaultAgency): the main agency when the user may see it, else their first
// by name. /agencies already comes back narrowed to the user's agencies and in name order,
// so "the first one" here is the first one there.
export function defaultAgencyId(agencies: Agency[] | null | undefined): string | undefined {
  const list = agencies ?? []
  return (list.find((agency) => agency.is_default) ?? list[0])?.id
}
