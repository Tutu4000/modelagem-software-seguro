# Etapa 4 — Código Seguro e Testes de Segurança

Esta etapa leva para o código Go duas decisões da [Etapa 3][e3]. Cada prática segue a ordem da seção 21 do enunciado: risco e requisito, testes, implementação, resultado esperado e referência da OWASP. Os testes são escritos e commitados antes da implementação, como pede a seção 21.1, e o histórico do git mostra essa ordem.

O código fica em [codigo/etapa-4/](../codigo/etapa-4/), e o plano completo está no [backlog](../codigo/etapa-4/BACKLOG.md). Os testes rodam com:

```bash
cd codigo/etapa-4
go test ./...
```

## Prática 1 — Controle de autorização por objeto

### Risco e requisito

* **Risco:** [R14] (crítico). Um usuário baixa o STL de outro cliente trocando o identificador na URL ([CA01]).
* **Requisito:** [RS01]. A identidade vem só do token. Antes de gerar a URL pré-assinada, a API confere se o arquivo pertence ao usuário. A URL vale 5 minutos, e o armazenamento não tem nenhum objeto público.
* **Decisão e controles:** [D01], com [C01] (dono do objeto), [C02] (UUID) e [C03] (URL pré-assinada).

O acesso do técnico atribuído ao pedido, também previsto no RS01, passa pelo binário `painel` e entra com o épico E6 do backlog.

### Testes definidos antes da implementação

|Teste|Entrada ou ação|Resultado esperado|
|-|-|-|
|TS01|O cliente A pede a URL do próprio STL e a usa|200 com a URL, que entrega o arquivo|
|TS02|O cliente A pede o STL do cliente B, também com `?user_id=B`, e pede um arquivo que não existe|403 nos três casos, com a mesma resposta para o arquivo alheio e para o inexistente. Cada tentativa vira um evento `authz.denied` no log, sem o token|
|TS03|Requisição sem token, com token malformado, assinado por outra chave, expirado, sem assinatura (`alg: none`) ou com o usuário trocado no payload|401|
|TS04|A URL é usada com 4min59s e com 5min01s|200 e depois 403|
|TS05|A URL é usada sem assinatura, com a assinatura alterada, com a validade estendida e no lugar de outro arquivo|403, sem nenhum conteúdo|

O TS01 é o caso válido, e os demais são casos maliciosos, inválidos ou não autorizados. O código está em [pratica1_autorizacao_test.go](../codigo/etapa-4/tests/pratica1_autorizacao_test.go).

## Prática 2 — Validação do upload de STL

### Risco e requisito

* **Risco:** [R20] (crítico). Muitos modelos de alta densidade enviados ao mesmo tempo sobrecarregam a renderização, o fatiamento e a análise DFM ([CA07]).
* **Requisito:** [RS03]. Antes de gravar o arquivo e de criar a tarefa, a API confere o formato real do STL pela estrutura do arquivo, e não pela extensão: no STL binário, o tamanho precisa ser `84 + 50 * n`, sendo `n` o número de triângulos do cabeçalho. Também confere o tamanho máximo de 100 MB e o limite de 2 milhões de triângulos.
* **Decisão e controle:** [D03], com [C10] (limites de upload).

Duas definições completam o requisito:

* O limite de 100 MB é de 100 MiB (104.857.600 bytes). Assim cabe um STL com 2 milhões de triângulos (100.000.084 bytes), e um arquivo íntegro com mais triângulos é barrado pela regra dos triângulos, e não pela do tamanho.
* Só o STL binário é aceito, porque a validação do RS03 é feita pela estrutura binária. O STL em texto (ASCII) é recusado.

### Testes definidos antes da implementação

|Teste|Entrada ou ação|Resultado esperado|
|-|-|-|
|TS06|Upload de um STL binário íntegro com 12 triângulos|202 com o estado "em processamento" e uma tarefa com o dono, o hash SHA-256 e o número de triângulos|
|TS07|Arquivo de 150 MB, com e sem `Content-Length`|413 e nada gravado nem enfileirado|
|TS08|PDF enviado como STL, STL em texto (ASCII), STL truncado, STL com bytes sobrando e arquivo menor que o cabeçalho|422 e nada gravado nem enfileirado|
|TS09|STL binário íntegro com 2.000.001 triângulos|422 e nada gravado nem enfileirado|

O TS06 é o caso válido, e os demais são casos maliciosos ou inválidos. O código está em [pratica2_upload_test.go](../codigo/etapa-4/tests/pratica2_upload_test.go).

A implementação, o resultado esperado e a referência da OWASP das duas práticas entram depois, no commit da implementação.

[e3]: etapa-3-arquitetura-segura.md
[R14]: etapa-2-riscos-nist.md#22-registro-de-riscos
[CA01]: etapa-1-ameacas-stride.md#ca01--roubo-de-projeto-confidencial
[RS01]: etapa-3-arquitetura-segura.md#1-requisitos-de-segurança
[D01]: etapa-3-arquitetura-segura.md#d01--o-servidor-decide-autorização-por-objeto-em-um-ponto-único
[C01]: etapa-2-riscos-nist.md#c01
[C02]: etapa-2-riscos-nist.md#c02
[C03]: etapa-2-riscos-nist.md#c03
[R20]: etapa-2-riscos-nist.md#22-registro-de-riscos
[CA07]: etapa-1-ameacas-stride.md#ca07--sobrecarga-do-processamento-de-modelos
[RS03]: etapa-3-arquitetura-segura.md#1-requisitos-de-segurança
[D03]: etapa-3-arquitetura-segura.md#d03--processamento-assíncrono-com-workers-isolados-e-falha-segura
[C10]: etapa-2-riscos-nist.md#c10
