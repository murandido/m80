---
name: lefthook-setup
description: Setup e troubleshooting do hook local (lefthook + goimports + golangci-lint + gitleaks). Use ao configurar um clone novo, quando o commit falhar com "go.mod requires go >= ...", ou quando o hook não rodar ("Can't find lefthook in PATH").
---

# Hook local (lefthook)

- Setup obrigatório uma vez por clone:
  ```bash
  go install github.com/evilmartians/lefthook@latest
  go install golang.org/x/tools/cmd/goimports@latest
  go install github.com/zricethezav/gitleaks/v8@v8.30.1
  lefthook install
  ```
- **Seu Go local precisa ser pelo menos o do `go.mod`.** O hook roda `golangci-lint`, que carrega os pacotes com o toolchain da máquina; se ele for mais velho, o commit falha com `go.mod requires go >= 1.x.y (running 1.x.z; GOTOOLCHAIN=local)` — e o mesmo acontece com o `air` dentro do container de dev, que passa a rodar binário velho em silêncio depois de um rebuild falho. O Go do Fedora vem com `GOTOOLCHAIN=local`, então ele não baixa a versão certa sozinho: ou instale o Go exigido, ou rode `go env -w GOTOOLCHAIN=auto`. Depois de subir a versão, **reconstrua as imagens de dev** (`docker compose --profile dev up -d --build`) — a imagem antiga não compila mais.
- **Garanta que `$(go env GOPATH)/bin` está no seu `PATH`** (ex: `~/.zshrc`/`~/.bashrc`). Se o binário `lefthook` não for encontrado no `PATH` no momento do commit, o hook gerado só imprime `"Can't find lefthook in PATH"` e deixa o commit passar **sem rodar nada** — falha silenciosa, sem bloquear.
- **gitleaks** varre o que está staged atrás de secrets antes do commit. Sem o binário, o passo avisa e deixa passar: o job `Secrets` do CI é o gate de verdade, e o hook é só o aviso antecipado. Achado que não é secret (chave de teste, identificador público) entra no `.gitleaksignore` pelo fingerprint que o gitleaks imprime, com o motivo; se for secret de verdade, primeiro rotacione.
