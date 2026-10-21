# M-80 Frontend - Diretrizes Next.js

> **⚠️ O painel está no ar desde 08/08/2026** (na Vercel até 29/08/2026, hoje num container ao lado da API), usado por gente de verdade para registrar emissão e venda. Como aqui **não há teste automatizado nenhum** (ver *Testes*), a rede é você: toda mudança de tela precisa ser exercitada à mão antes de ir, e o `pnpm build` é o único gate automático que existe.
> - **Painel e API sobem juntos.** Desde a migração as duas imagens são publicadas pela mesma label `deploy:vps`, então não existe mais a janela em que o painel ia ao ar antes do backend. O `BACKEND_URL` é o nome do serviço no Compose, não uma URL pública.
> - **Autosave e reconciliação de linha na grid de Emissões são caminho crítico.** É onde uma regressão apaga o que o operador acabou de digitar, em silêncio. A regra de ouro em *Tempo Real (SSE)* não é estilo.

## Stack Base
- Next.js 15/16+ (App Router Exclusivamente).
- Tailwind CSS 4 + `shadcn/ui` para componentes UI.
- Validação: `zod 4.3.6` com `react-hook-form` + `@hookform/resolvers/zod`.

## Estrutura de Pastas
```
src/
├── actions/      # Server Actions para mutações (POST/PUT/DELETE)
├── app/          # Rotas e layouts (App Router)
├── components/   # Componentes organizados por domínio (auth/, users/, layout/, ui/)
├── hooks/        # Custom React hooks
├── lib/          # Utilitários: env.ts, utils.ts (cn())
├── schemas/      # Schemas Zod centralizados
└── proxy.ts      # Middleware de invalidação de cookie JWT
```

## Regras de Arquitetura e Código
1. **Server vs Client Components:** Por padrão, tudo é Server Component. Só use `"use client"` em componentes de folha (leaf components) que precisam de interatividade (`useState`, `onClick`, hooks). Nunca marque um componente pai como `"use client"` só para passar dados.
2. **Formulários:** O padrão absoluto do projeto é `react-hook-form` acoplado com `@hookform/resolvers/zod`.
3. **Validação (Zod):** Os schemas ficam centralizados em `src/schemas/`. Reutilize-os entre form e Server Action.
4. **Data Fetching e Mutações:**
   - Buscas (GET) devem ser feitas diretamente nos Server Components (`page.tsx`) aproveitando o cache nativo.
   - Mutações (POST/PUT/DELETE) devem usar Server Actions em `src/actions/`.
5. **Estilização:** Use o utilitário `cn()` de `src/lib/utils.ts` para composição condicional de classes. Nunca concatene strings de classe manualmente.
6. **Imports:** Use sempre caminhos absolutos via `@/`:
   ```ts
   import { cn } from "@/lib/utils"
   import { LoginSchema } from "@/schemas/auth"
   ```
7. **Imagens:** Use sempre `next/image` — nunca `<img>` diretamente.
8. **Nomenclatura de arquivos de componente:** use kebab-case (ex: `app-sidebar.tsx`, `create-user-form.tsx`), consistente com os componentes existentes. Novos primitivos de UI (shadcn) são adicionados via `pnpm dlx shadcn add <componente>`, que já os coloca em `src/components/ui/` no estilo/tema corretos.

## Listas longas (virtualização)
- `@tanstack/react-virtual` (versão exata no `package.json`) é a ferramenta para lista que pode crescer sem limite. Hoje é usada só no controle de pagamentos (`components/sales/sales-clients-table.tsx` + `use-sales-window.ts`), sobre `GET /sales/page`.
- O modelo é o de um chat (WhatsApp/Discord), por pedido do usuário: a lista abre na venda mais nova, só um trecho contínuo de até 4 páginas fica em memória e **a rolagem cobre só esse trecho** (a barra não representa o histórico inteiro). Chegar perto do topo traz a página anterior acima e solta a do fim, e vice-versa. Quando entra ou sai página no topo, a linha que estava na tela é recolocada no mesmo lugar (âncora por id da venda), e os gatilhos só ligam depois que a abertura assentou no fim.
- Linha virtualizada é medida (`measureElement`) e chaveada pelo id da venda (`getItemKey`), para manter a altura medida quando entram páginas acima. A estimativa (`ESTIMATED_ROW_PX`) e os skeletons têm a altura real da linha larga (53px); se o layout da linha mudar, ajuste os dois juntos, senão a abertura não para exatamente no fim. Estado de edição de uma linha (rascunho) não pode morar só nela: ela desmonta quando sai de vista, então a linha em edição é mantida montada e a página dela não é descartada.

## Autenticação
- Token JWT reside no cookie `auth_token`.
- `src/proxy.ts` — invalida o cookie e redireciona em caso de 401.
- Layouts verificam sessão ativa chamando `/auth/me` no backend.
- Não leia o token diretamente em Client Components; acesse via Server Component ou Server Action.
- **O backend Go não lê cookies.** Ao chamar a API (Server Action ou Server Component), leia o token do cookie e anexe manualmente como header `Authorization: Bearer <token>` em cada `fetch` — não existe um client HTTP central que faça isso automaticamente.
- **O browser nunca fala direto com o backend Go.** `BACKEND_URL` é server-only e o Go exige o header `Authorization`, que nem `EventSource` nem um `<a download>` conseguem enviar. Toda chamada que precisa partir do cliente passa por um Route Handler em `src/app/api/`, que lê o cookie, converte em Bearer e repassa a resposta (ver `api/issuances/export`, `api/issuances`, `api/events`). Nunca exponha o backend via `NEXT_PUBLIC_*` nem coloque o token na URL.
- As rotas `src/app/api/extension/*` servem a extensão de navegador, não o painel: aceitam a sessão **só** como `Authorization: Bearer` (nunca o cookie, para não haver credencial automática que outro site possa usar), respondem CORS apenas para origens de extensão e passam pelo `relayToApi`/`answerPreflight` de `src/lib/extension-relay.ts`. Rota nova para a extensão segue o mesmo molde.

## Tempo Real (SSE)
- `useSSE` (`src/hooks/use-sse.ts`) abre a conexão com `/api/events` e despacha por tipo de evento. É agnóstico de domínio — reutilize-o em vez de criar outro `EventSource`.
- Reconexão é a nativa do `EventSource`. **Não** implemente retry manual.
- O `EventSource` não reenvia o que foi publicado enquanto estava desconectado (deploy, queda de rede). Todo consumidor que refaz busca em evento deve passar `{ onReconnect: refresh }` ao `useSSE`, senão a tela fica velha até o F5 depois de cada reconexão.
- Ao reagir a um evento, refaça a busca com os filtros atuais em vez de mesclar o payload: as regras de filtro moram no backend e duplicá-las em TypeScript gera divergência silenciosa.
- Regra de ouro ao aplicar dado do servidor na grid de Emissões: nunca sobrescreva linha em rascunho, salvando, com erro, ou em edição (`reconcile-rows.ts`). O autosave segue a mesma regra — uma resposta atrasada não pode apagar o que o operador digitou desde então.

## Variáveis de Ambiente
- `NEXT_PUBLIC_*` — expostas ao browser (use com cautela, nunca para secrets).
- Sem prefixo — disponíveis apenas server-side (Server Components, Server Actions, Route Handlers).
- Acesse via `src/lib/env.ts` (não use `process.env` diretamente fora desse arquivo).
- Variáveis atuais (ver `.env.sample`): `NODE_ENV`, `BACKEND_URL`. Nenhuma `NEXT_PUBLIC_*` em uso hoje.

## Testes
- Não há testes automatizados configurados neste projeto (sem Jest/Vitest/Playwright/Testing Library). Novas features não exigem testes de frontend, a menos que solicitado explicitamente.

## Adicionando uma Página ao Dashboard
1. Crie a rota em `src/app/dashboard/<feature>/page.tsx`.
2. Registre o link **dentro de uma das seções** de `navSections`, em `src/lib/navigation.ts` (Operação / Gestão / Dev Mode). Esse arquivo é fonte de verdade de três coisas ao mesmo tempo — a sidebar, o breadcrumb do header e a busca `⌘K` — então nenhum outro arquivo precisa mudar.
   - **A permissão é por link, não por seção**: cada item aceita `roles`. `roles: ["admin"]` libera admin e dev; `roles: []` é só dev; ausente é todo mundo. A seção some sozinha quando nada nela é alcançável. (Este doc dizia que a restrição era da seção, via `adminOnly` — o código deixou de funcionar assim quando "Gestão" passou a misturar telas de operador com telas de admin.) O backend repete a restrição nas próprias rotas, e a página precisa do seu próprio `requireAdmin()`/`requireDev()`: esconder o link não é controle de acesso.
   - Se a rota tiver segmentos abaixo do link (ex: `/dashboard/sales/suppliers`), acrescente o rótulo em `nestedSegmentLabels` de `src/components/layout/nav-breadcrumb.tsx`. Segmentos não mapeados são omitidos do breadcrumb de propósito, para ids não aparecerem crus.
