# M-80 Backend - Domínio de Dados (GORM)

> **⚠️ Há um banco de produção com dado real desde 08/08/2026.** Este é o pacote onde um erro custa mais caro, porque é o único que apaga coisa. Antes de escrever migration ou mexer em seed:
> - **Migration destrutiva** (`DROP`, `ALTER ... TYPE`, `NOT NULL` em coluna existente, renomear coluna) → `make db-backup` antes, `down` que funciona de verdade, e **fale com o usuário antes de escrever o arquivo**. Ela roda sozinha no boot da API e o rollback do deploy troca imagem, não schema.
> - **Coluna nova em tabela cheia**: nascer nullable ou com default. `NOT NULL` sem default trava a subida da API em produção e não trava nada em dev, onde a tabela está vazia.
> - **Seed reconciliador** agora corrige linhas que alguém já usou. A lista do que ele nunca toca (mais abaixo) não é formalidade — é o que impede o seed de reescrever dado de operação.
> - Modelo **sem** `gorm.DeletedAt` sofre hard delete de verdade. Em produção isso é definitivo; a recuperação parcial está em `docs/backup.md` e depende do dump da noite anterior.

> **Schema documentado:** a seção *Schema do Banco de Dados* do Documento de Decisões e Requisitos (Notion, link em `backend/carriers/CLAUDE.md`) descreve todas as tabelas campo a campo, com o motivo de cada escolha (índices únicos parciais, `complete` nas entidades auto-criáveis, separação entre `cancelled` e `deleted`, dinheiro em centavos). As migrations em `migrations/` continuam sendo a fonte de verdade; o documento é o "porquê". Ao criar uma migration, atualize a tabela correspondente lá.

## Modelos
- Todo modelo novo implementa `BeforeCreate(tx *gorm.DB) error` para gerar o UUID da primary key quando vazio:
  ```go
  func (p *Person) BeforeCreate(tx *gorm.DB) error {
      if p.ID == "" {
          p.ID = uuid.New().String()
      }
      return nil
  }
  ```
- Campos opcionais são ponteiros (`*string`, `*time.Time`), não `sql.NullString`/`sql.NullTime`.

## Soft Delete
- **Nem todo modelo tem soft delete.** Hoje têm `DeletedAt gorm.DeletedAt`: `User`, `Issuance`, `Account`, `AccountNickname` e `LoyaltyProgram`. Os demais (`Person`, `Agency`, `Airport`, `Airline`, etc.) não têm — `h.DB.Delete(...)` neles executa um **hard delete** de verdade.
- Só adicione `gorm.DeletedAt` a um modelo quando o produto precisar disso — não adicione por padrão/simetria. Os dois motivos que já valeram:
  - **Lixeira** (`User`, `Issuance`) — o registro precisa poder voltar.
  - **Arquivamento forçado pela FK** (`Account`, `AccountNickname`, `LoyaltyProgram`, migration 29) — a linha é referenciada com `ON DELETE RESTRICT` por uma tabela que **ela própria** é soft-deletada, então o hard delete nunca mais passa. Aqui o soft delete não é lixeira: é a única forma de a linha sumir do painel sem apagar a que emissão antiga ela pertencia. Não existe tela de restaurar.
- **Soft delete + `Preload` se cancelam.** O escopo do GORM também vale nos precarregamentos, então preload de um modelo arquivado devolve `nil` e a história fica sem rótulo. Onde a associação existe para **contar o passado**, precarregue com `Unscoped()` — ver `preloadIssuanceAssociations` em `internal/api/v1/handlers/issuances.go`. Onde ela existe para **oferecer uma escolha**, deixe o escopo agir.
- **Índice único + soft delete também se cancelam**, no outro sentido: a linha arquivada continua ocupando o nome. Se a coluna única for um nome que o operador pode querer reusar (o `code` do programa, o apelido da conta), o índice tem que virar parcial com `WHERE deleted_at IS NULL`.
- Modelos referenciados por outra tabela via FK `ON DELETE RESTRICT` (ex: `people` é referenciada por `accounts.holder_id`/`administrator_id`) vão falhar o delete no Postgres se ainda houver referência. Trate isso no handler, não deixe vazar um `500` cru: capture `*pgconn.PgError` (`github.com/jackc/pgx/v5/pgconn`) e cheque `err.Code == "23503"` para responder `422` com uma mensagem amigável — ver `internal/api/v1/handlers/people.go` (`DeletePerson`) como referência.

## Campos cifrados em repouso

Três colunas guardam texto cifrado (AES-256-GCM, `internal/utils/crypto.go`), sempre pelo mesmo par de hooks `BeforeSave`/`AfterFind` no modelo: `accounts.password`, `issuance_passengers.document` e `people.cpf`. Quem lê pelo GORM recebe o valor em claro; quem abre o banco vê `enc:...`.

Duas consequências que não são óbvias:

- **`utils.Decrypt` devolve intacto o que não tem o prefixo `enc:`.** É isso que permite migrar uma coluna gradualmente, em vez de num único corte: linha antiga continua legível enquanto o backfill não chegou nela (ver `backfill_person_cpf.go`).
- **O nonce é aleatório, então índice único sobre a coluna cifrada não vale nada** — o mesmo valor cifra diferente toda vez. Onde a unicidade importa, ela mora num *blind index* ao lado: `people.cpf_fingerprint` e `account_beneficiaries.cpf_fingerprint`, ambos HMAC-SHA256 sob `CPF_INDEX_KEY` (`internal/utils/blindindex.go`). Os dois usam a mesma chave e a mesma normalização de propósito — é o que faz o titular de uma conta ser reconhecido sem que nenhum dos lados guarde o número.

## Migrations
- Ver seção "Banco de Dados e Migrations" em `backend/CLAUDE.md` — convenção de nomes e fluxo do golang-migrate estão documentados lá, não duplicados aqui.

## Seeds
- Dados de referência que precisam existir no banco (não são cadastro do usuário) seguem o padrão `seed_<tabela>.go` na raiz de `internal/data/`, com o JSON fonte em `internal/data/seeds/<tabela>.json` (embutido via `//go:embed`).
- Seeds são chamados explicitamente em `cmd/api/main.go`, **depois** de `data.Connect()` — nunca dentro de `data.Connect()` nem via hook do GORM. O `/cmd/bot` não semeia nada; só lê. Falha de seed é `log.Fatal`: subir com catálogo pela metade é pior que não subir.
- **Ordem importa quando há FK entre seeds.** `SeedAirlines` roda antes de `SeedLoyaltyPrograms` porque `loyalty_programs.default_airline_id` é `NOT NULL` e é resolvido pelo `code` da cia.

### Dois modos, e como escolher
Todo seed é idempotente, mas por caminhos diferentes:

- **Só quando vazio** (`seed_airports.go`) — conta as linhas e não faz nada se houver qualquer uma, com `ON CONFLICT DO NOTHING` como segunda camada. É o certo para dado que é só **volume inicial**: ninguém digita aeroporto à mão, e se digitou, não está errado.
- **Reconciliador** (`seed_airlines.go`) — casa cada registro do JSON com a linha existente e **corrige as colunas de que o catálogo é dono**, preservando o uuid. É o certo para dado que espelha um **catálogo externo** e que pode estar errado no banco: as cias foram digitadas à mão sem `traveos_airline_id`, com códigos que não batiam com os logos do bilhete e com um nome com typo.

O reconciliador exige três coisas, e nenhuma é opcional:
1. **Lista explícita de `aliases`** no JSON (códigos antigos, nomes legais, typos conhecidos). Casar por alias declarado é auditável; um normalizador esperto adivinha.
2. **Uma linha de log por mudança.** Seeder que conserta dado em silêncio é impossível de explicar depois.
3. **Uma lista escrita do que ele nunca toca.** Em `seed_airlines.go` são o `traveos_supported` de `true` para `false` (só promove, nunca rebaixa), a `brand_color` já preenchida (a vendor é a fonte) e qualquer cia que não esteja no JSON. Preservar o uuid é inegociável — as FKs apontam para ele.

Os flags `requires_last_name`/`requires_purchase_number` **são** reconciliados, nos dois sentidos, e isso é deliberado: quais cias exigem sobrenome ou número da compra é fato sobre a companhia, não preferência de instalação. Errar nos dois sentidos custa — restrito demais recusa uma EMISSAO que estava certa, frouxo demais gasta uma emissão real e não-idempotente numa requisição que a vendor não consegue responder.

`seed_loyalty_programs.go` é o meio-termo deliberado: casa por code/nome/alias, mas **não altera nada** do que encontra. `accounts.loyalty_program_id` aponta para lá, e programa duplicado espalha as contas de um programa entre duas linhas.
