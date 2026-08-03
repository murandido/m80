"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Backpack, Luggage, Pencil } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"
import { Spinner } from "@/components/ui/spinner"
import { IssuanceFlight } from "@/schemas/issuance"
import {
  IssuanceBaggage,
  IssuancePassenger,
  IssuanceSeat,
} from "@/schemas/issuance-services"
import {
  putIssuanceBaggageAction,
  putIssuanceSeatsAction,
} from "@/actions/issuance-services"

// The editor lives beside the ticket, never inside it. The document is rendered
// in a sandboxed iframe precisely so its stylesheet cannot reach the dashboard,
// and it is also the thing the passenger receives - not a form.
//
// Saving refreshes the route, which re-renders the document on the server, so
// the preview and the downloadable PDF move together. That is the property the
// whole ticket package exists to keep: one generator, never two layouts.

interface TicketServicesPanelProps {
  issuanceID: string
  flights: IssuanceFlight[]
  passengers: IssuancePassenger[]
  seats: IssuanceSeat[]
  baggage: IssuanceBaggage[]
}

const DIRECTION_LABEL: Record<string, string> = {
  outbound: "Ida",
  inbound: "Volta",
}

// The two classes that exist on a real fare, in the vendor's own array order:
// the cabin bag first, the checked bag second. The weight is not an editable
// field because it is not a free choice - it is the class - and a text box
// invites "23 kg" and "23kgs", which the ticket would then print verbatim.
//
// The sync still writes whatever the airline sent, weights we have never seen
// included. This list is what the panel *offers*, not what the column *accepts*.
const BAGGAGE_CLASSES = [
  { position: 0, weight: "10kg", label: "Mão", Icon: Backpack },
  { position: 1, weight: "23kg", label: "Despachada", Icon: Luggage },
] as const

export function TicketServicesPanel({
  issuanceID,
  flights,
  passengers,
  seats,
  baggage,
}: TicketServicesPanelProps) {
  const router = useRouter()
  const [isOpen, setIsOpen] = useState(false)
  const [isPending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)

  // Draft state, seeded from the server on every open. Re-seeding on open is
  // what stops a panel left open through a Traveos re-sync from writing back a
  // version of the booking that no longer exists.
  const [draftSeats, setDraftSeats] = useState<IssuanceSeat[]>([])
  const [draftBaggage, setDraftBaggage] = useState<IssuanceBaggage[]>([])

  const legs = [...flights].sort((a, b) =>
    a.direction === b.direction
      ? a.leg_order - b.leg_order
      : a.direction === "outbound"
        ? -1
        : 1,
  )
  const directions = ["outbound", "inbound"].filter((direction) =>
    legs.some((leg) => leg.direction === direction),
  )

  function handleOpenChange(open: boolean) {
    if (open) {
      setError(null)
      // One row per passenger per leg, whether or not the airline gave a seat.
      // An absent row is not a gap the operator should have to notice.
      setDraftSeats(
        passengers.flatMap((passenger) =>
          legs.map((leg) => ({
            passenger_id: passenger.id,
            flight_id: leg.id,
            seat:
              seats.find(
                (seat) => seat.passenger_id === passenger.id && seat.flight_id === leg.id,
              )?.seat ?? null,
          })),
        ),
      )
      // Both classes always, for every direction, whether or not the airline
      // reported them. A missing row is "none of this class", which is a
      // quantity of zero - not a row the operator should have to conjure.
      setDraftBaggage([
        ...passengers.flatMap((passenger) =>
          directions.flatMap((direction) =>
            BAGGAGE_CLASSES.map((klass) => ({
              passenger_id: passenger.id,
              direction: direction as IssuanceBaggage["direction"],
              position: klass.position,
              weight: klass.weight,
              quantity:
                baggage.find(
                  (allowance) =>
                    allowance.passenger_id === passenger.id &&
                    allowance.direction === direction &&
                    allowance.position === klass.position,
                )?.quantity ?? 0,
            })),
          ),
        ),
        // Anything the airline reported beyond the two classes rides along
        // untouched. The PUT replaces the whole set, so leaving it out of the
        // draft would delete a real entitlement to make room for a form that
        // does not know how to show it.
        ...baggage.filter(
          (allowance) =>
            !BAGGAGE_CLASSES.some((klass) => klass.position === allowance.position),
        ),
      ])
    }
    setIsOpen(open)
  }

  function updateSeat(passengerID: string, flightID: string, value: string) {
    setDraftSeats((current) =>
      current.map((seat) =>
        seat.passenger_id === passengerID && seat.flight_id === flightID
          ? { ...seat, seat: value.trim() === "" ? null : value.toUpperCase() }
          : seat,
      ),
    )
  }

  function quantityOf(passengerID: string, direction: string, position: number) {
    return (
      draftBaggage.find(
        (allowance) =>
          allowance.passenger_id === passengerID &&
          allowance.direction === direction &&
          allowance.position === position,
      )?.quantity ?? 0
    )
  }

  function updateQuantity(
    passengerID: string,
    direction: string,
    position: number,
    quantity: number,
  ) {
    setDraftBaggage((current) =>
      current.map((allowance) =>
        allowance.passenger_id === passengerID &&
        allowance.direction === direction &&
        allowance.position === position
          ? { ...allowance, quantity }
          : allowance,
      ),
    )
  }

  function handleSave() {
    setError(null)
    startTransition(async () => {
      const savedSeats = await putIssuanceSeatsAction(issuanceID, draftSeats)
      if ("error" in savedSeats) {
        setError(savedSeats.error)
        return
      }

      const savedBaggage = await putIssuanceBaggageAction(issuanceID, draftBaggage)
      if ("error" in savedBaggage) {
        setError(savedBaggage.error)
        return
      }

      setIsOpen(false)
      // Re-renders the document on the server, so the preview and the PDF the
      // button downloads are the same edit.
      router.refresh()
    })
  }

  return (
    <Sheet open={isOpen} onOpenChange={handleOpenChange}>
      <SheetTrigger asChild>
        <Button variant="outline" size="sm">
          <Pencil className="size-4" />
          Assentos e bagagens
        </Button>
      </SheetTrigger>

      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>Assentos e bagagens</SheetTitle>
          <SheetDescription>
            O que a companhia mudou depois da emissão. O bilhete e o PDF são
            regerados com o que você salvar aqui.
          </SheetDescription>
        </SheetHeader>

        <div className="flex flex-col gap-6 px-4 pb-4">
          {passengers.length === 0 && (
            <p className="text-muted-foreground text-sm">
              Essa emissão ainda não tem passageiros. Repuxe da Traveos antes de
              corrigir assentos.
            </p>
          )}

          {passengers.map((passenger) => (
            <div key={passenger.id} className="flex flex-col gap-3">
              <h3 className="text-sm font-semibold">
                {passenger.first_name} {passenger.last_name}
              </h3>

              <div className="flex flex-col gap-2">
                <Label className="text-muted-foreground text-xs">Assentos</Label>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                  {legs.map((leg, index) => {
                    const inputID = `seat-${passenger.id}-${leg.id}`
                    const legNumber =
                      legs.filter(
                        (other, otherIndex) =>
                          other.direction === leg.direction && otherIndex <= index,
                      ).length
                    return (
                      <div key={leg.id} className="flex flex-col gap-1">
                        <Label htmlFor={inputID} className="text-muted-foreground text-xs">
                          {DIRECTION_LABEL[leg.direction]} · T{legNumber}
                          {leg.origin_iata_code && leg.destination_iata_code
                            ? ` · ${leg.origin_iata_code}–${leg.destination_iata_code}`
                            : ""}
                        </Label>
                        <Input
                          id={inputID}
                          value={
                            draftSeats.find(
                              (seat) =>
                                seat.passenger_id === passenger.id &&
                                seat.flight_id === leg.id,
                            )?.seat ?? ""
                          }
                          onChange={(event) =>
                            updateSeat(passenger.id, leg.id, event.target.value)
                          }
                          placeholder="12A"
                          maxLength={8}
                          disabled={isPending}
                        />
                      </div>
                    )
                  })}
                </div>
              </div>

              {directions.map((direction) => (
                <div key={direction} className="flex flex-col gap-2">
                  <Label className="text-muted-foreground text-xs">
                    Bagagens · {DIRECTION_LABEL[direction]}
                  </Label>

                  <div className="grid grid-cols-2 gap-2">
                    {BAGGAGE_CLASSES.map(({ position, weight, label, Icon }) => {
                      const inputID = `qty-${passenger.id}-${direction}-${position}`
                      return (
                        <div key={position} className="flex items-end gap-2">
                          <Icon className="text-muted-foreground mb-2.5 size-4 shrink-0" />
                          <div className="flex flex-1 flex-col gap-1">
                            <Label htmlFor={inputID} className="text-muted-foreground text-xs">
                              {label} · {weight}
                            </Label>
                            <Input
                              id={inputID}
                              type="number"
                              min={0}
                              max={99}
                              value={quantityOf(passenger.id, direction, position)}
                              onChange={(event) =>
                                updateQuantity(
                                  passenger.id,
                                  direction,
                                  position,
                                  Number(event.target.value) || 0,
                                )
                              }
                              disabled={isPending}
                            />
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </div>
              ))}

              <Separator />
            </div>
          ))}

          <p className="text-muted-foreground text-xs">
            Quantidade zero aparece no bilhete — é a companhia oferecendo e o
            passageiro não tendo comprado. Os pesos são fixos porque são a
            classe da franquia, não uma medida: a bagagem de mão é a de 10kg e a
            despachada é a de 23kg.
          </p>

          {error && <p className="text-destructive text-sm">{error}</p>}
        </div>

        <SheetFooter>
          <Button onClick={handleSave} disabled={isPending || passengers.length === 0}>
            {isPending && <Spinner />}
            Salvar
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
