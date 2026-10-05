# Página de Emissões — referência técnica (handoff)

Documento-resumo da página `/dashboard/issuances` para quando a UI for refeita: o que ela
faz, como se comporta e **o que precisa ser preservado** num redesign. A implementação vive
em `frontend/src/components/issuances/` (a rota em si só busca dados).

## Na `ca-mode`: board de cards + painel (protótipo)
Nesta branch a tela não é a planilha: é um **board de cards** com um **painel lateral de formulário**
(tela cheia no celular), desenhado pelo designer. O código vive em `components/issuances/board/`.
Os arquivos da planilha continuam no repositório, sem uso, para os merges da main não conflitarem.
Tudo abaixo desta seção descreve a planilha.

- **Ponto de troca:** `components/issuances/issuances-screen.tsx` renderiza `IssuanceBoardScreen`.
  O layout da página é um grid só (`board/board-layout.module.css`). O título e a busca vêm do
  `page.tsx`, o resto vem do board, e `:has(.panel)` abre espaço para o painel.
- **Estado:** `board/use-issuance-board.ts`, portado do grid (rascunho → POST, fila de patches
  durante o POST, PATCH otimista, SSE + `reconcileRows`, criação inline de conta, cancelar,
  excluir, sync). Ficaram de fora a máquina de célula, a colagem e o undo global.
- **Prévia antes dos valores:** com "Autocompletar dados" ligado, um **rascunho** com cia que
  autocompleta e os campos de `hasAirlineMinimumFields` chama `POST /issuances/lookup`
  (`backend/internal/issuance/lookup.go`). A prévia consulta os provedores na mesma ordem e com
  as mesmas regras do `Sync`, **sem gravar a emissão**. A Traveos pode importar. Ela preenche
  origem, destino, data e passageiro, e os valores nunca travam. Depois do save, o sweep de sync
  grava o itinerário de verdade. Esse sweep nunca roda ao lado de uma prévia da mesma linha (com
  a Traveos seriam duas importações) e pula a combinação que a prévia já viu falhar.
- **"Cia que autocompleta"** é `direct_booking_enabled || (traveos_supported && traveos_airline_id)`,
  igual ao `resolveProviders` do backend. O `autocomplete.ts` da planilha só conhece a metade da
  Traveos.
- **Teclado:** Ctrl+Enter abre uma nova emissão. Enter confirma o campo e vai ao próximo. As setas
  andam entre os cards e o Esc minimiza o painel (voltando o foco ao card). `/` no Programa pula
  para a Cia, e no localizador do programa pula para o da cia. Ctrl+Alt+C e Ctrl+Alt+X copiam a
  mensagem e o cálculo, e Ctrl+Shift+⌫ exclui.
- **Divergências conscientes do design:** o link "Visualizar informações completas/simplificadas"
  sempre nomeia a *outra* visão. Conta aparece na visão do autocompletar, mas não dispara a busca.
  "Último nome" só aparece quando a cia exige. A cor do card é a do programa, e cancelado fica
  cinza. Data da emissão, Nº da compra, taxa de resgate e status manual foram encaixados no
  formulário.
- **Simular Emissão:** calculadora de milheiro (pontos, milheiro e taxas → total + linha de
  cálculo). Não salva nada.

## Propósito
Uma **planilha turbinada** para cadastrar emissões de milhas **rápido, só com teclado** —
edição inline célula a célula, com autosave. Deve continuar parecendo e operando como uma
planilha (Excel/Sheets), não como um formulário.

## Layout
- **Toolbar** (topo): botão **Atalhos** (popover com a legenda de teclas) + **Nova emissão**.
- **Grade** numa região de scroll com **cabeçalho fixo**; coluna `#` à esquerda com o número
  da linha + **bolinha de autosave**; coluna de ações (lixeira) à direita ao passar o mouse.
- Linhas **canceladas** e com **erro de salvamento** aparecem com fundo vermelho.
- **Célula ativa** é destacada por um anel (`ring`); a seleção rola pra dentro da viewport.

## As 14 colunas
| # | Rótulo | Campo(s) no backend | Tipo de célula / regra |
|---|--------|---------------------|------------------------|
| 0 | Cia aérea | `loyalty_program_id` (+ `airline_id`) | Combobox com badge colorido. `/` sobrepõe a cia default do programa. |
| 1 | Localizador | `program_locator` (+ `airline_locator`) | Texto; `/` separa localizador do programa e da cia. Exibe `PNR (cia)`. |
| 2 | Data do voo | `flight_date` (ISO) | Máscara `DD/MM/AAAA`. |
| 3 | Passageiro principal | `main_passenger_name` | Texto. |
| 4 | Origem | `origin_iata_code` | Autocomplete de aeroporto (busca por código/nome/cidade). |
| 5 | Destino | `destination_iata_code` | Autocomplete de aeroporto. |
| 6 | Pontos | `points` | Inteiro em **milheiros**: vírgula = ×1000 (`0,5` → 500; exibe `0.500`). |
| 7 | Valor milheiro | `value_of_the_thousand` | Moeda **BRL em centavos**. |
| 8 | Taxas | `taxes` | Moeda **BRL em centavos**. |
| 9 | Valor total | — | **Calculado, somente leitura** (pontos × valor + taxas). Pulado na navegação. |
| 10 | Conta | `account_id` (+ `account_nickname_id`) | Combobox filtrado pelo programa, com **criação inline** de conta. |
| 11 | Emissor | `issuer_id` | Combobox (usuários). Default: usuário logado. |
| 12 | Agência | `agency_id` | Combobox (agências). |
| 13 | Status | `status` | Select colorido (Incompleto/Completo/Cancelado). |

## Modelo de interação (teclado-first)
- **Uma célula ativa** por vez (anel de seleção).
- **Entrar em edição:** `F2`, clique, ou **começar a digitar** (substitui o valor).
- **Confirmar e mover:** `Enter` (baixo), `Shift+Enter` (cima), `Tab` (direita),
  `Shift+Tab` (esquerda). Ao passar do fim, **cria uma nova emissão** automaticamente.
- **Sair da edição:** `Esc` (descarta), sair do campo/`blur` (salva).
- A coluna calculada e linhas canceladas são **puladas** na navegação por Tab/Enter.

### Atalhos completos (também na legenda "Atalhos")
| Tecla | Ação |
|-------|------|
| Setas | Mover a seleção |
| `F2` / digitar | Editar a célula |
| `Enter` / `Shift+Enter` | Confirmar e ir pra célula abaixo / acima |
| `Tab` / `Shift+Tab` | Confirmar e ir pra próxima / anterior |
| `Home` / `End` | Início / fim da linha |
| `Ctrl+Home` / `Ctrl+End` | Início / fim da grade |
| `Ctrl` + setas | Saltar até a borda |
| `Ctrl+Enter` | Nova emissão |
| `Delete` | Limpar a célula |
| `Ctrl+C` / `Ctrl+V` | Copiar valor exibido (toast) / colar (entra em edição com o texto) |
| `Ctrl+Alt+C` | Copiar a linha como mensagem EMISSAO (o mesmo do ícone de copiar) |
| `Ctrl+Alt+X` | Copiar só a linha de cálculo (`31k * 16,5 + 189,22 tx = 700,72`) |
| `Ctrl+Z` / `Ctrl+Shift+Z` (ou `Ctrl+Y`) | Desfazer / refazer (**só valores de célula**) |
| `Ctrl+Shift+⌫` | Excluir a linha (com confirmação) |
| `/` | Separar `programa/cia` (Cia aérea, Localizador) |

## Persistência (autosave)
- **Rascunho** (linha sem `id`): vive só no client até os **5 campos obrigatórios**
  (`program_locator`, `loyalty_program_id`, `account_id`, `points`, `value_of_the_thousand`)
  estarem preenchidos → **POST**. Edições feitas durante o POST em voo são enfileiradas e
  reaplicadas como PATCH quando o `id` chega.
- **Existente:** cada edição de célula é um **PATCH otimista** (aplica local e salva).
- **Bolinha de autosave** por linha: idle / salvando / ok / erro; falha deixa a linha
  vermelha até um salvamento seguinte dar certo.
- **Excluir:** DELETE (rascunho é só removido do estado local).
- `status` é **derivado pelo backend** (é o único campo re-hidratado da resposta).
- Não há cliente HTTP central: o token JWT do cookie é anexado como `Authorization: Bearer`
  em cada chamada (via server actions em `src/actions/`).

## Dados de referência
As listas dos comboboxes (programas de fidelidade, cias aéreas, contas (+apelidos),
agências, usuários, pessoas) são carregadas em paralelo no **layout** do route group
`(grid)/layout.tsx` e chegam à grade por contexto (`IssuancesReferenceProvider`, em
`components/issuances/issuances-screen.tsx`). O `page.tsx` só busca as **emissões** e o
`currentUserId`: layout não re-renderiza quando só os search params mudam, então uma troca
de filtro custa uma consulta, não sete. Aeroportos vêm à parte, de `/api/airports`.
Cada lista tolera falha isolada. Se as **emissões** falharem, a grade não é renderizada
(evita recadastro duplicado); se só listas de referência falharem, um alerta avisa que
campos de seleção podem vir incompletos.

## Comportamentos especiais (fáceis de perder num redesign)
- **`/` na Cia aérea:** programa antes da barra, cia (override do default) depois. A cia
  default do programa fica implícita (não é mostrada).
- **Troca de programa reseta a conta** (`account_id`/apelido → null) e realça a célula Conta
  em vermelho até uma conta válida ser escolhida (conta pertence a um único programa).
- **Criação inline de conta** na célula Conta (cria pessoa + conta + apelido); exige o
  programa preenchido antes.
- **Foco invisível:** um `<textarea>` oculto segura o foco fora de edição para capturar o
  `Ctrl+V` nativo e as teclas de navegação; por isso a seleção precisa rolar pra dentro da
  tela explicitamente.

## Autocompletar dados (Traveos)
Toggle na barra de filtros (preferência em `localStorage`, `issuances:autocomplete`). Ligado, a
grade puxa o itinerário da Traveos sozinha na **primeira linha candidata** — uma de cada vez, uma
tentativa por linha por carregamento de página (`isAutocompleteCandidate` + `autocompleteAttemptedRef`).

O mínimo para disparar **varia por companhia**, e por isso os cabeçalhos das colunas que ainda
faltam ficam **vermelhos enquanto a linha estiver selecionada** (`autocompleteHintFor`), com o
motivo no `title`:

| Sempre | Só se a cia exigir |
|---|---|
| Localizador (do programa ou da cia) | Origem (`requires_origin` — GOL, Azul) |
| Pontos, Valor milheiro, Conta | Passageiro principal (`requires_last_name`) |
| | Nº da compra (`requires_purchase_number`) |

- **Taxas não entra** — é anulável, a linha salva sem ela.
- **Pontos / Valor milheiro / Conta** não são exigências da Traveos: são o que
  `draftIsReadyToCreate` pede para a linha ser salva, e o autocomplete só olha para linhas salvas.
- **Nº da compra não tem coluna** — é digitado depois de um `/` na célula Localizador, então
  acende aquele cabeçalho e o `title` explica.
- **Cia sem integração** (`traveos_supported` off ou sem `traveos_airline_id`): nenhum vermelho, e
  um aviso âmbar na toolbar — preencher tudo não faria a linha autocompletar.
- **Origem só para quem a lê** (`airlines.requires_origin`, reconciliado pelo seed): entre as
  integrações, só GOL e Azul acham a reserva pela origem. Nessas, o backend não a valida e mandaria
  `""`. Sem ela, o autocomplete simplesmente nunca dispara, sem erro nenhum. É o caso que mais
  justifica o realce.
- **Sobrenome para a busca:** `issuances.passenger_last_name`, quando preenchido, é o sobrenome
  mandado para a cia. Se não, vale o `main_passenger_name` (`data.Issuance.BookingLastName`). O
  sync sobrescreve o passageiro principal com o nome completo, mas nunca toca nesse campo.

## Performance
Hoje a grade usa **um único contexto React**; mover a seleção re-renderiza todas as células
(aceitável em volumes pequenos/médios). **Follow-up planejado (PR próprio):** dividir o
contexto (estável vs. volátil) + `React.memo` para re-renderizar só as células afetadas, e
eventualmente virtualizar linhas. Um redesign **não deve piorar** isso.

## O que é visual (livre) vs. comportamento (preservar)
- **Pode redesenhar à vontade:** cores, espaçamento, tipografia, estilo de badges / inputs /
  dropdowns / bolinha de autosave, layout da toolbar, aparência do anel de seleção e da
  legenda.
- **Precisa preservar:** modelo de 1 célula ativa + edição inline; **toda a navegação por
  teclado**; autosave otimista por célula e seus estados; o **parsing** (pontos em milheiros,
  moeda em centavos, máscara de data, sintaxe `/`); criação inline de conta; reset de conta
  ao trocar de programa; undo/redo; copiar/colar; coluna Valor total somente leitura;
  distinção rascunho vs. existente.

## Mapa de arquivos (para quem for reimplementar)
- `components/issuances/issuances-grid.tsx` — estado, teclado, autosave/persist, render.
- `components/issuances/grid-cell.tsx` — dispatch coluna → componente de célula.
- `components/issuances/cells/*` — uma célula por tipo (texto, número, data, localizador,
  aeroporto, cia aérea, combobox, status, calculada, portal do dropdown).
- `components/issuances/interaction-reducer.ts` — máquina de estado (seleção/edição/sticky).
- `lib/grid-navigation.ts` — navegação pura (próxima célula, clamp, constantes de coluna).
- `components/issuances/column-fields.ts` — patch de limpeza por coluna (tecla Delete).
- `components/issuances/types.ts` — shape da linha (`IssuanceRow`) e helpers de rascunho.
- `components/issuances/grid-context.tsx` — contexto compartilhado.
- `app/dashboard/issuances/(grid)/page.tsx` — busca das emissões e resolução dos filtros.
- `app/dashboard/issuances/(grid)/layout.tsx` — listas de referência dos comboboxes.
- `actions/issuance.ts`, `actions/account.ts` — mutações (POST/PATCH/DELETE).
