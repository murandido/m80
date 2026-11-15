# Página de Emissões — referência técnica (handoff)

Documento-resumo da página `/dashboard/issuances` para quando a UI for refeita: o que ela
faz, como se comporta e **o que precisa ser preservado** num redesign. A implementação vive
em `frontend/src/components/issuances/` (a rota em si só busca dados).

## A tela atual: board de cards + painel
A tela não é mais a planilha: é um **board de cards** com um **painel lateral de formulário**
(tela cheia no celular), desenhado pelo designer na `ca-mode`. O código vive em `components/issuances/board/`.
A planilha antiga foi removida do código. A descrição dela (colunas, atalhos, modelo de
célula) ficou no histórico: veja este arquivo no commit `a132caf`.

- **Ponto de troca:** `components/issuances/issuances-screen.tsx` renderiza `IssuanceBoardScreen`.
  O layout da página é um grid só (`board/board-layout.module.css`). O título e a busca vêm do
  `page.tsx`, o resto vem do board, e `:has(.panel)` abre espaço para o painel.
- **Estado:** `board/use-issuance-board.ts`, portado do grid (rascunho → POST, fila de patches
  durante o POST, PATCH otimista, SSE + `reconcileRows`, criação inline de conta, cancelar,
  excluir, sync). Ficaram de fora a máquina de célula, a colagem e o undo global.
- **Prévia antes dos valores:** com "Autocompletar dados" ligado, um **rascunho** com cia que
  autocompleta e os campos de `hasLookupFields` (`board/use-issuance-board.ts`) chama `POST /issuances/lookup`
  (`backend/internal/issuance/lookup.go`). A prévia consulta os provedores na mesma ordem e com
  as mesmas regras do `Sync`, **sem gravar a emissão**. A Traveos pode importar. Ela preenche
  origem, destino, data e passageiro, e os valores nunca travam. Depois do save, o sweep de sync
  grava o itinerário de verdade. Esse sweep nunca roda ao lado de uma prévia da mesma linha (com
  a Traveos seriam duas importações) e pula a combinação que a prévia já viu falhar.
- **Campos da busca por cia:** origem só onde `airlines.requires_origin` está ligado (GOL e Azul),
  e o campo **Sobrenome** (`issuances.passenger_last_name`) só na visão simplificada, com o
  autocompletar ligado numa cia com `requires_last_name`. O Sobrenome é o que a cia recebe (`BookingLastName`); o Passageiro
  principal fica com o nome completo que o sync devolve.
- **Rascunhos** ficam no `localStorage` (`issuances:drafts`) e voltam depois de um F5. O aviso de
  "sair da página" só aparece com um save em andamento. São por navegador, e duas abas com
  rascunhos diferentes regravam a mesma chave.
- **"Cia que autocompleta"** é `direct_booking_enabled || (traveos_supported && traveos_airline_id)`,
  igual ao `resolveProviders` do backend. O `autocomplete.ts` da planilha só conhece a metade da
  Traveos.
- **Teclado:** Ctrl+Enter abre uma nova emissão. Enter confirma o campo e vai ao próximo. As setas
  andam entre os cards e o Esc minimiza o painel (voltando o foco ao card). `/` no Programa pula
  para a Cia, e no localizador do programa pula para o da cia. Ctrl+Alt+C e Ctrl+Alt+X copiam a
  mensagem e o cálculo, e Ctrl+Shift+⌫ exclui.
- **Divergências conscientes do design:** o link "Visualizar informações completas/simplificadas"
  sempre nomeia a *outra* visão. Conta aparece na visão do autocompletar, mas não dispara a busca.
  A cor do card é a do programa, e cancelado fica cinza. O card é quadrado a partir de duas
  colunas; no celular ele ocupa a largura toda. Data da emissão, Nº da compra e status manual foram encaixados no
  formulário.
- **Simular Emissão:** calculadora de milheiro (pontos, milheiro e taxas → total + linha de
  cálculo). Não salva nada.
- **Alteração ≠ correção.** Editar Pontos/Milheiro/Taxas no painel (ou a `ALTERACAO` do bot) *corrige*
  o que foi digitado errado e reescreve a emissão. Quando a **passagem muda depois de emitida** (a cia
  cobra milhas ou taxa a mais por trocar um trecho, ou devolve algo), o caminho é a seção
  **Alterações** do painel (`board/adjustments-section.tsx` + `board/adjustment-dialog.tsx`), que grava
  um movimento datado em `issuance_adjustments` (`POST/PATCH/DELETE /issuances/{id}/adjustments`).
  Cada movimento diz se é repassado ao cliente (milheiro próprio + taxas) ou absorvido/retido pela
  agência. A venda soma o `client_amount_cents` dos movimentos, o custeio FIFO consome ou devolve na
  data do movimento e o precifica pelo milheiro dele, e a conciliação com o extrato conta o débito
  extra no mesmo localizador. Os valores da emissão continuam sendo a reserva como foi feita. A linha do painel
  aberto é protegida do refresh do SSE, então o hook (`saveAdjustment`/`deleteAdjustment`) aplica a
  resposta direto na linha. Emissão cancelada não aceita alteração.
  A alteração pertence ao **dia em que aconteceu**. A emissão antiga entra no fluxo desse dia, no
  de quem registrou (terceiro ramo de `applyIssuanceListFilters`), e a planilha de controle escreve só a
  linha da alteração (`ExportWindow` em `internal/export`). Quando a alteração muda o que o cliente deve,
  a venda também aparece no controle de pagamentos desse dia.

## Persistência (autosave)
- **Rascunho** (linha sem `id`): vive só no client até os **5 campos obrigatórios**
  (`program_locator`, `loyalty_program_id`, `account_id`, `points`, `value_of_the_thousand`)
  estarem preenchidos → **POST**. Edições feitas durante o POST em voo são enfileiradas e
  reaplicadas como PATCH quando o `id` chega.
- **Existente:** cada edição de campo é um **PATCH otimista** (aplica local e salva).
- **Bolinha de autosave** por linha: idle / salvando / ok / erro; falha deixa a linha
  vermelha até um salvamento seguinte dar certo.
- **Excluir:** DELETE (rascunho é só removido do estado local).
- `status` é **derivado pelo backend** (é o único campo re-hidratado da resposta).
- Não há cliente HTTP central: o token JWT do cookie é anexado como `Authorization: Bearer`
  em cada chamada (via server actions em `src/actions/`).

## Dados de referência
As listas dos comboboxes (programas de fidelidade, cias aéreas, contas (+apelidos),
agências, usuários, pessoas) são carregadas em paralelo no **layout** do route group
`(grid)/layout.tsx` e chegam ao board por contexto (`IssuancesReferenceProvider`, em
`components/issuances/issuances-screen.tsx`). O `page.tsx` só busca as **emissões** e o
`currentUserId`: layout não re-renderiza quando só os search params mudam, então uma troca
de filtro custa uma consulta, não sete. Aeroportos vêm à parte, de `/api/airports`.
Cada lista tolera falha isolada. Se as **emissões** falharem, o board não é renderizado
(evita recadastro duplicado); se só listas de referência falharem, um alerta avisa que
campos de seleção podem vir incompletos.

## Mapa de arquivos
- `components/issuances/issuances-screen.tsx` — ponto de entrada; renderiza `IssuanceBoardScreen`.
- `components/issuances/board/use-issuance-board.ts` — estado, autosave, SSE, lookups.
- `components/issuances/board/issuance-board.tsx`, `issuance-panel.tsx`, `board-fields.tsx` — cards, painel e campos.
- `components/issuances/board/number-parsing.ts` — leitura de pontos e moeda nos campos.
- `components/issuances/types.ts` — shape da linha (`IssuanceRow`) e helpers de rascunho.
- `components/issuances/grid-context.tsx` — listas de referência compartilhadas.
- `components/issuances/emissao-message.ts` — total da emissão e a mensagem EMISSAO copiada.
- `actions/issuance.ts`, `actions/account.ts` — mutações (POST/PATCH/DELETE).
