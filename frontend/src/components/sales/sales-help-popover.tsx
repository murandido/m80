"use client"

import { HelpCircle } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

// The rules on this screen that nobody guesses by looking at it: where a lump sum from a
// client goes, what the yellow bar means, how to take a change back, and what cancelling a
// booking does to what the client already paid. Same shape as the "Atalhos" popover on the issuances grid, which
// is where that screen keeps the rules its layout cannot show.
const SECTIONS = [
  {
    title: "Situação",
    lines: [
      "Um clique no botão alterna entre Pago e A Pagar.",
      "Voltar para A Pagar apaga só os pagamentos lançados pelo botão — o que veio de um pagamento do cliente continua lá.",
      "Emissão paga só por pagamento do cliente não volta pelo botão: desfaça o pagamento no Histórico.",
    ],
  },
  {
    title: "Pagamento parcial",
    lines: [
      "Na barra de baixo, escolha o cliente e informe quanto ele pagou.",
      "O valor é abatido nas emissões dele que estão a pagar, da mais antiga para a mais nova — inclusive as fora do período na tela.",
      "A última emissão alcançada fica parcial: a barra amarela mostra quanto dela já foi pago.",
      "Pagou a mais? A sobra vira crédito do cliente e é abatida sozinha na próxima emissão vinculada a ele.",
      "Trocar o cliente de uma emissão paga devolve o dinheiro ao cliente antigo e abate o crédito do novo.",
    ],
  },
  {
    title: "Desfazer",
    lines: [
      "Toda mudança mostra um aviso com Desfazer por alguns segundos.",
      "Ctrl+Z desfaz a última mudança da tela (fora dos campos de texto).",
      "Histórico, na barra de baixo, lista os pagamentos do cliente e onde cada um foi abatido; um valor errado se desfaz ali a qualquer momento.",
    ],
  },
  {
    title: "Cancelamento",
    lines: [
      "Cancelar o bilhete não mexe no que foi lançado pelo botão — o dinheiro está no caixa até alguém devolver.",
      "O que veio de um pagamento do cliente sai da emissão cancelada e vai para a próxima dele, ou vira crédito.",
      "Uma emissão cancelada que tinha sido paga mostra Devolver; o clique registra a devolução.",
      "Cancelou e ninguém tinha pago? Não há nada a fazer, e a linha aparece como Cancelada.",
    ],
  },
  {
    title: "A faixa colorida",
    lines: [
      "Verde: pago. Amarelo: pago em parte. Vermelho: a pagar.",
      "Cinza: cancelada, ou emissão ainda sem valor.",
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
