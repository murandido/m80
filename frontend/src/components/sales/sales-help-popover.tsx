"use client"

import { HelpCircle } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

// The two rules on this screen that nobody guesses by looking at it: that a sale settles
// itself once the entries add up, and that cancelling a booking does not touch what the
// client already paid. Same shape as the "Atalhos" popover on the issuances grid, which is
// where that screen keeps the rules its layout cannot show.
const SECTIONS = [
  {
    title: "Pagamentos",
    lines: [
      "A venda é quitada quando os lançamentos somam o valor dela — não há mais interruptor de pago.",
      "Recebeu em partes? Lance cada parte com o dia em que o dinheiro entrou; a coluna Saldo mostra o que falta.",
      "“Receber tudo” lança o valor cheio de uma vez.",
      "Lançou errado? Apague o lançamento — o saldo volta sozinho.",
    ],
  },
  {
    title: "Devolução",
    lines: [
      "Para devolver dinheiro ao cliente, lance um valor negativo.",
      "O saldo volta a zero e fica registrado quando foi e quem lançou.",
    ],
  },
  {
    title: "Cancelamento",
    lines: [
      "Cancelar o bilhete não mexe no que o cliente já pagou — o dinheiro está no caixa até alguém devolver.",
      "O que muda é a dívida: um bilhete cancelado não é mais cobrável, então a venda passa a mostrar o que há para devolver.",
      "Cancelou e ninguém tinha pago? Não há nada a fazer, e a linha aparece como Sem pendência.",
      "Os pontos e as taxas que voltaram da companhia, e a multa, ficam na emissão — esta tela é só o dinheiro entre a agência e o cliente.",
    ],
  },
  {
    title: "A faixa colorida",
    lines: [
      "Âmbar na borda esquerda: ainda tem dinheiro para entrar ou para sair.",
      "Verde: nada pendente — quitada, ou cancelada sem dinheiro envolvido.",
    ],
  },
]

export function SalesHelpPopover() {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline">
          <HelpCircle className="h-4 w-4" />
          Como funciona
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96">
        <div className="space-y-3">
          {SECTIONS.map((section) => (
            <div key={section.title}>
              <p className="mb-1 text-sm font-medium">{section.title}</p>
              <ul className="space-y-1">
                {section.lines.map((line) => (
                  <li key={line} className="text-muted-foreground text-xs">
                    {line}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}
