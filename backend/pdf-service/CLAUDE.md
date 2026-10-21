# pdf-service

Microserviço Python que expõe um endpoint HTTP interno para geração de PDF via WeasyPrint.

> **⚠️ Em produção desde 08/08/2026**, como imagem **arm64** publicada no GHCR (`ghcr.io/murandido/m80-pdf-service`) e rodando numa Oracle Ampere A1. Ele **não** publica porta e só é alcançável pela rede interna do Compose — não adicione `ports:` no perfil prod para debugar; firewall do host não protege porta publicada por container, porque o Docker escreve as regras dele antes. Quando ele cai, o bilhete simplesmente não chega no WhatsApp do cliente.

## Responsabilidade
- Recebe HTML via `POST /pdf` (corpo `text/html`)
- Retorna bytes do PDF (`application/pdf`)
- É consumido internamente pelo `api-dev` / `api-prod` — não é exposto publicamente
- **Não busca recurso externo nenhum.** O `url_fetcher` só aceita `data:`: imagem, CSS ou fonte por `http(s)://` ou `file://` é descartada pelo WeasyPrint com um aviso no log, sem quebrar o PDF. É de propósito — fecha SSRF e leitura de arquivo local na raiz. Todo documento precisa ser autocontido (SVG inline, `data:` URI, fontes do sistema); um `<img src="https://...">` novo some do PDF em silêncio.

## Stack
- Python 3.12
- Flask (servidor HTTP)
- WeasyPrint (renderização HTML → PDF)

## Comandos locais (fora do Docker)
```bash
pip install -r requirements.txt
python app.py
```

## Porta padrão
5000
