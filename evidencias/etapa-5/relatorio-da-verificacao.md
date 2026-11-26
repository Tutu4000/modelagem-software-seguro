# Relatório da verificação — evidências

Este arquivo reúne as evidências brutas da [Etapa 5](../../docs/etapa-5-verificacao-vulnerabilidades.md): onde está cada relatório, como repetir a verificação e as saídas que sustentam os achados A01 a A05. A análise está no documento da etapa.

## 1. Arquivos

|Pasta ou arquivo|Conteúdo|
|-|-|
|[relatorios-zap/](relatorios-zap/)|Rodada 1 (07/11/2026): a API, em HTML, JSON e Markdown|
|[relatorios-zap-fe0/](relatorios-zap-fe0/)|Rodada 2 (08/11/2026): a API e o app do cliente, com a análise no [LEIAME](relatorios-zap-fe0/LEIAME.md)|
|[relatorios-zap-fe7/](relatorios-zap-fe7/)|Rodada 3 (09/11/2026): o sistema completo, com a análise no [LEIAME](relatorios-zap-fe7/LEIAME.md) e o plano em [zap.yaml](relatorios-zap-fe7/zap.yaml)|
|[relatorios-zap-dirigido/](relatorios-zap-dirigido/)|Rodada 4 (14/11/2026): varredura dirigida da API e do painel interno, com o método e o resultado no [LEIAME](relatorios-zap-dirigido/LEIAME.md)|
|[capturas-de-tela/](capturas-de-tela/)|Capturas dos relatórios HTML do ZAP|

|Captura|O que mostra|
|-|-|
|[zap-rodada-1-api.png](capturas-de-tela/zap-rodada-1-api.png)|Rodada 1: os alertas de HSTS ausente e de versão do servidor exposta (A01)|
|[zap-rodada-3-api.png](capturas-de-tela/zap-rodada-3-api.png)|Rodada 3, API: o alerta alto de SQL Injection (A02) e os demais|
|[zap-rodada-3-app-cliente.png](capturas-de-tela/zap-rodada-3-app-cliente.png)|Rodada 3, app do cliente|
|[zap-rodada-4-dirigida.png](capturas-de-tela/zap-rodada-4-dirigida.png)|Rodada 4: a API e o painel, sem o alerta de SQL Injection|

## 2. Como repetir

Em `codigo/etapa-4/`, com Docker e Go instalados:

```sh
just secrets                      # gera segredos e certificado locais
just up                           # sobe o ambiente
just zap out=relatorios-zap-novo  # roda as duas varreduras do ZAP
go test ./...                     # testes unitários, inclusive os dos achados A03 e A04
```

## 3. Resumo dos alertas do ZAP por rodada

|Alerta|Risco|Rodada 1|Rodada 2|Rodada 3|
|-|-|-|-|-|
|SQL Injection (40018)|Alto|—|5|2|
|Strict-Transport-Security Header Not Set (10035)|Baixo|3|—|—|
|Server Leaks Version Information (10036)|Baixo|3|—|—|
|Unexpected Content-Type was returned (100001)|Baixo|15|44|46|
|Insufficient Site Isolation Against Spectre (90004)|Baixo|—|—|3|
|ZAP is Out of Date (10116)|Baixo|1|—|2|
|A Client Error response code was returned (100000)|Informativo|25|64|64|
|Demais informativos (cache, comentários e SPA)|Informativo|3|9|29|

A rodada 1 varreu só a API. As rodadas 2 e 3 somam a API e o app do cliente. A rodada 4, dirigida, tem a própria tabela no [LEIAME](relatorios-zap-dirigido/LEIAME.md): um alerta médio de CSP, nenhum de SQL Injection e 16 respostas `500` que não viraram alerta.

## 4. Evidências por achado

### A01 — Cabeçalhos ausentes (ZAP, rodada 1)

Trecho de [relatorios-zap/zap-relatorio.json](relatorios-zap/zap-relatorio.json):

```text
Server Leaks Version Information via "Server" HTTP Response Header Field
    GET  https://nginx:8443/api/v1/arquivos/id            evidência: nginx/1.30.6
    GET  https://nginx:8443/api/v1/arquivos/id/download   evidência: nginx/1.30.6
    POST https://nginx:8443/api/v1/arquivos               evidência: nginx/1.30.6
Strict-Transport-Security Header Not Set
    as mesmas três rotas
```

Correção, em `codigo/etapa-4/deploy/nginx/`:

```nginx
server_tokens off;
add_header Strict-Transport-Security "max-age=31536000" always;
```

Os dois alertas não aparecem nas rodadas 2 e 3.

### A02 — SQL Injection, falso positivo (ZAP, rodada 3)

Trecho de [relatorios-zap-fe7/zap-relatorio.json](relatorios-zap-fe7/zap-relatorio.json):

```text
POST https://nginx:8443/api/v1/tickets            parâmetro: subject
    ataque: "... AND 1=1 -- " e "... AND 1=2 -- "
POST https://nginx:8443/api/v1/webhooks/payments  parâmetro: event_id
    ataque: "John Doe AND 1=1 -- " e "John Doe OR 1=1 -- "
```

A explicação, com a reprodução manual, está no [LEIAME da rodada 2](relatorios-zap-fe0/LEIAME.md#sql-injection-por-que-é-falso-positivo) e no [LEIAME da rodada 3](relatorios-zap-fe7/LEIAME.md).

### A03 — Estouro de inteiro no preço (revisão manual)

Teste `TestHugeVolumeIsRefusedInsteadOfOverflowing`, rodado contra o código anterior à correção:

```text
--- FAIL: TestHugeVolumeIsRefusedInsteadOfOverflowing
    volume 1e+20:   quote {UnitCents:1500000000000000000 DiscountCents:27970680741345361 TotalCents:5785759348784974743}
    volume 4.6e+21: quote {UnitCents:-9223372036854775808 DiscountCents:0 TotalCents:1000}
    volume 1e+30:   quote {UnitCents:-9223372036854775808 DiscountCents:0 TotalCents:1000}
```

O `TotalCents:1000` é o total de R$ 10,00, só a taxa de setup.

Teste pela interface, na noite de 09/11/2026 (já 10/11 em UTC), com a checagem retirada de propósito e um STL com um vértice a 2,36 × 10²¹ mm (volume de 2,14 × 10²³ mm³, quantidade 2). Log da API:

```text
00:23:57 POST /api/v1/orders 500
ERROR: new row for relation "orders" violates check constraint "orders_shipping_cents_check" (SQLSTATE 23514)
00:24:03 POST /api/v1/orders 500   (o mesmo erro)
00:24:04 POST /api/v1/orders 500   (o mesmo erro)
```

O mesmo pedido, com a correção:

```text
POST /api/v1/orders 400
{"type":"/errors/invalid_request","title":"Invalid request","status":400,"code":"invalid_request"}
log: invalid_request: the model is too big to quote: a unit above 100000000000 cents
```

Os horários do log são UTC.

### A04 — Negação de serviço pelo upload (revisão manual)

Testes de `codigo/etapa-4/tests/upload_memory_test.go`, rodados contra o código anterior à correção:

```text
--- FAIL: TestParallelUploadsAreBoundedInMemory
    status 422, want 503
--- FAIL: TestMismatchedContentLengthIsRefusedWithoutReading
    the API read 52428884 bytes of a body it could refuse from the header
```

* No primeiro, dois uploads de 100 MB ficam presos no meio do corpo, e um terceiro é aceito para leitura: não havia limite para os uploads simultâneos.
* No segundo, o corpo tem um `Content-Length` diferente do que o cabeçalho do STL declara, e a API leu os 50 MB antes de recusar.

Com a correção, os dois testes passam: o terceiro upload recebe `503`, e o corpo é recusado depois dos 84 bytes do cabeçalho.

### A05 — Byte nulo no endereço (ZAP dirigido, rodada 4)

Log da API e do painel durante a varredura:

```text
POST /api/v1/orders                      500  (8 vezes)
POST /panel/v1/orders/:id/address-change 500  (8 vezes)
ERROR: invalid byte sequence for encoding "UTF8": 0x00 (SQLSTATE 22021)
```

Teste `TestAddressWithControlCharactersIsRefused`, rodado contra o código anterior à correção:

```text
--- FAIL: TestAddressWithControlCharactersIsRefused
    --- FAIL: NUL_in_the_recipient
    --- FAIL: NUL_in_the_street
    --- FAIL: NUL_in_the_number
    --- FAIL: NUL_in_the_city
    --- FAIL: line_break_in_the_city
    --- FAIL: escape_in_the_recipient
```

Com a correção, o teste passa, e o mesmo pedido pela API recebe `400`.

## 5. Outras verificações da mesma sessão

|Ferramenta|Alvo|Resultado|
|-|-|-|
|`go vet`, `gosec` e `govulncheck`|Código Go|Nenhum achado|
|`gitleaks`|Histórico do git (63 commits)|Nenhum segredo|
|`npm audit`|Dependências do front-end|0 em produção; 10 moderadas só em ferramentas de desenvolvimento|
|Trivy|Imagens Docker|`api`, `panel` e `worker` sem achados. O `nginx` tinha 10 CVEs altas ou críticas, corrigidas com a atualização dos pacotes. A imagem base do Keycloak tem 8 nos pacotes do sistema, ainda pendentes. Na Etapa 7, o `just scan-images` sem cache mostrou que a correção do nginx tinha ficado incompleta (6 achados, pois a base fixa o pacote) e a refez, e contou 19 no Keycloak, que seguem em aberto (ver o [roteiro da Etapa 7](../../roteiros/etapa-7-devsecops-e-video-final.md#3-o-que-já-existe-e-o-que-falta))|
|Playwright|Os dois apps, num navegador de verdade|52 de 52 testes|
|Testes de ponta a ponta em Go|Ambiente completo|46 de 47. O que falha exige o gVisor, ausente no Docker Desktop do Windows|

## 6. Commits relacionados

|Commit|Conteúdo|
|-|-|
|[75ff909](https://github.com/Tutu4000/modelagem-software-seguro/commit/75ff909f1d8b7c9b51ec78d5c25b6958a9099719)|Os testes que reproduzem os achados da revisão manual, todos falhando|
|[a0cd7e8](https://github.com/Tutu4000/modelagem-software-seguro/commit/a0cd7e872446a33ba4f6368f2df0788ddd4fa821)|As correções, com os testes passando|
|[7832333](https://github.com/Tutu4000/modelagem-software-seguro/commit/783233325f087d9f766c3d261cf1ca024205d583)|A atualização dos pacotes da imagem do nginx|
