"use client"

import { useGridContext } from "@/components/issuances/grid-context"
import { TextCell } from "@/components/issuances/cells/text-cell"
import { LocatorCell } from "@/components/issuances/cells/locator-cell"
import { DateMaskCell } from "@/components/issuances/cells/date-mask-cell"
import { NumberCell } from "@/components/issuances/cells/number-cell"
import { CalculatedCell } from "@/components/issuances/cells/calculated-cell"
import { ComboboxCell } from "@/components/issuances/cells/combobox-cell"
import { AirportAutocompleteCell } from "@/components/issuances/cells/airport-autocomplete-cell"
import { CiaAereaCell } from "@/components/issuances/cells/cia-aerea-cell"
import { StatusCell } from "@/components/issuances/cells/status-cell"

export function GridCell({ r, c }: { r: number; c: number }) {
  const { accounts, users, agencies } = useGridContext()

  switch (c) {
    case 0:
      return <CiaAereaCell r={r} c={c} />
    case 1:
      return <LocatorCell r={r} c={c} />
    case 2:
      return <DateMaskCell r={r} c={c} />
    case 3:
      return <TextCell r={r} c={c} field="main_passenger_name" />
    case 4:
      return <AirportAutocompleteCell r={r} c={c} field="origin_iata_code" />
    case 5:
      return <AirportAutocompleteCell r={r} c={c} field="destination_iata_code" />
    case 6:
      return <NumberCell r={r} c={c} field="points" mode="integer" />
    case 7:
      return <NumberCell r={r} c={c} field="value_of_the_thousand" mode="currency" />
    case 8:
      return <NumberCell r={r} c={c} field="taxes" mode="currency" />
    case 9:
      return <CalculatedCell r={r} />
    case 10:
      return (
        <ComboboxCell
          r={r}
          c={c}
          field="account_id"
          options={accounts.map((a) => ({ value: a.id, label: a.login ?? a.id }))}
        />
      )
    case 11:
      return (
        <ComboboxCell
          r={r}
          c={c}
          field="issuer_id"
          options={users.map((u) => ({ value: u.id, label: u.name }))}
        />
      )
    case 12:
      return (
        <ComboboxCell
          r={r}
          c={c}
          field="agency_id"
          options={agencies.map((a) => ({ value: a.id, label: a.name }))}
        />
      )
    case 13:
      return <StatusCell r={r} c={c} />
    default:
      return null
  }
}
