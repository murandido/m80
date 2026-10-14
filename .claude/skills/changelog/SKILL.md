---
name: changelog
description: Gera o changelog do M80 para avisar a equipe no grupo de WhatsApp, cobrindo o que foi publicado em produção desde o último aviso. Use quando o usuário pedir o changelog, as "novidades" ou o aviso de um deploy.
---

# Changelog para a equipe (WhatsApp)

Quem lê é a equipe que usa o painel e o bot no dia a dia, não programadores. O texto tem que ser
entendido por qualquer pessoa: o que mudou **na tela** e o que isso muda **no trabalho** dela.

## 1. Descobrir o que entrou em produção

Produção só muda por deploy (label `deploy:vps` num PR mergeado). Cada run do `deploy-vps.yml`
publica a `main` inteira **do momento em que rodou** (job *Resolve main*). Por isso o que está no ar é
"tudo que foi mergeado na `main` até o último deploy que deu certo", e não só o PR que levou a label.

1. O último deploy bem-sucedido:
   ```bash
   gh run list --workflow deploy-vps.yml --status success --limit 10 \
     --json databaseId,createdAt,displayTitle
   ```
   Se houver um deploy **em andamento**, espere terminar ou avise o usuário. Não anuncie o que
   ainda não está no ar.
2. O início do período é o fim do último changelog anunciado (a data fica na memória do Claude:
   procure a memória `changelog-last-announced`). Sem esse registro, **pergunte ao usuário desde
   quando**, sugerindo os deploys listados no passo 1 com data e PR.
3. Os PRs que entram, ou seja, os mergeados na `main` entre o início e o `createdAt` do último deploy
   bem-sucedido:
   ```bash
   gh pr list --state merged --base main --limit 50 \
     --search "merged:>=<AAAA-MM-DDTHH:MM:SSZ>" \
     --json number,title,body,mergedAt,labels
   ```
   Descarte os com `mergedAt` posterior ao último deploy: estão na `main`, mas não no ar.
4. Leia **título e corpo** de cada PR. O título em Conventional Commits só dá a pista; o que vira
   texto para a equipe é o efeito descrito no corpo.

## 2. Classificar

| Seção | Entra | Exemplos de prefixo |
|---|---|---|
| *Novidades* | algo que a equipe passa a poder fazer ou ver | `feat` |
| *Correções* | algo que estava errado ou atrapalhava e agora funciona | `fix` |
| *Saiu do sistema* | tela, botão, campo ou fluxo que deixou de existir | remoções explícitas no corpo do PR |

**Não entra**, por decisão do usuário: o que só muda nos bastidores e ninguém vê nem sente. Isso
inclui testes, documentação (`docs`), dependências (`chore(deps)`), refatoração, CI, lint, logs,
ajustes de performance invisíveis e mudanças só no bot de WhatsApp, que está pausado.

Um PR pode virar mais de um item (uma novidade e uma correção) ou se juntar a outro quando os dois
descrevem a mesma coisa para quem usa. Se nada entrar numa seção, a seção não aparece.

## 3. Escrever

- **Português simples, frases curtas.** Fale do que a pessoa vê e faz: "Na tela de Pagamentos, agora
  dá para…". Nada de nome técnico: API, endpoint, migration, SSE, FIFO, backend, PR, deploy, cache,
  virtualização.
- **O nome do sistema é "M80", sem hífen**, em todo texto para a equipe.
- **Diga onde fica**, usando o nome que aparece no menu ou na tela: *Emissões*, *Controle de
  pagamentos*, *Fornecedores*, *Contas*, o painel lateral do card etc.
- **Diga o efeito no dia a dia**, não a implementação. Exemplo ruim: "o saldo de venda cancelada
  passou a considerar a multa". Exemplo bom: "Quando uma emissão é cancelada com multa, a multa
  agora aparece para o cliente pagar, e a devolução já vem descontada".
- **Direto ao ponto, por pedido do usuário.** Um item por mudança, **uma frase só** sempre que der
  (duas no máximo): o que mudou e onde clicar. Sem explicação de motivo, sem contexto de "antes
  era assim", sem frase de efeito. Se duas mudanças são a mesma coisa para quem usa, viram um item.
- **Se mudou um número que já estava na tela** (saldo, total, valor), avise com uma frase do tipo
  "pode ser que alguns valores antigos apareçam diferentes; é a correção".
- Formatação do WhatsApp: `*negrito*` só nos títulos das seções e nos nomes de tela, e `•` nos
  itens. Sem tabelas, links, `#`, crases ou markdown de outro tipo. **Sem emojis**, a pedido do
  usuário.

## 4. Formato da mensagem

```
*Atualização do M80 — DD/MM*

*Novidades*
• *Emissões:* …
• *Controle de pagamentos:* …

*Correções*
• …

*Saiu do sistema*
• …
```

A data é a do último deploy incluído, no fuso de São Paulo.

## 5. Entregar

1. Mostre ao usuário o texto pronto, num bloco de código, para ele copiar sem a formatação do
   terminal estragar os `*`.
2. Logo abaixo, fora do bloco, liste os PRs considerados, com número e título, e o que ficou de fora
   por ser bastidor. Isso serve para ele conferir; não vai para a equipe.
3. **Não envie a mensagem para lugar nenhum.** O usuário cola no grupo.
4. Depois de ele confirmar que vai usar, grave ou atualize a memória `changelog-last-announced` com
   o `createdAt` do último deploy incluído. É o ponto de partida do próximo changelog.
