# Etapa 4 — Código Seguro e Testes de Segurança

Esta etapa leva para o código Go duas decisões da [Etapa 3][e3]. Cada prática segue a ordem da seção 21 do enunciado: risco e requisito, testes, implementação, resultado esperado e referência da OWASP. Os testes são escritos e commitados antes da implementação, como pede a seção 21.1, e o histórico do git mostra essa ordem.

O código fica em [codigo/etapa-4/](../codigo/etapa-4/), e o plano completo está no [backlog](../codigo/etapa-4/BACKLOG.md). Os testes rodam com:

```bash
cd codigo/etapa-4
go test ./...
```

## Respostas de erro

Todo erro da API segue o formato Problem Details ([RFC 9457]), com `Content-Type: application/problem+json`:

```json
{"type": "/errors/invalid_stl", "title": "Invalid STL", "status": 422, "code": "invalid_stl"}
```

|Código|Status|Quando|
|-|-|-|
|`unauthenticated`|401|Token ausente ou inválido|
|`forbidden`|403|Arquivo de outro cliente ou inexistente|
|`file_too_large`|413|Upload acima de 100 MiB|
|`invalid_stl`|422|Arquivo que não é um STL binário íntegro|
|`too_many_triangles`|422|STL com mais de 2 milhões de triângulos|
|`internal`|500|Qualquer erro não previsto|

A resposta traz só o código e um título fixo. A causa interna vai para o log e nunca para o cliente, como pedem a [D03] e o [Error Handling Cheat Sheet][cs-errors] da OWASP. Os testes do formato ficam em [problem_test.go](../codigo/etapa-4/internal/problem/problem_test.go).

## Prática 1 — Controle de autorização por objeto

### Risco e requisito

* **Risco:** [R14] (crítico). Um usuário baixa o STL de outro cliente trocando o identificador na URL ([CA01]).
* **Requisito:** [RS01]. A identidade vem só do token. Antes de gerar a URL pré-assinada, a API confere se o arquivo pertence ao usuário. A URL vale 5 minutos, e o armazenamento não tem nenhum objeto público.
* **Decisão e controles:** [D01], com [C01] (dono do objeto), [C02] (UUID) e [C03] (URL pré-assinada).

O acesso do técnico atribuído ao pedido, também previsto no RS01, passa pelo binário `panel` e entra com o épico E6 do backlog.

### Testes definidos antes da implementação

|Teste|Entrada ou ação|Resultado esperado|
|-|-|-|
|TS01|O cliente A pede a URL do próprio STL e a usa|200 com a URL, que entrega o arquivo|
|TS02|O cliente A pede o STL do cliente B, também com `?user_id=B`, e pede um arquivo que não existe|403 nos três casos, com a mesma resposta para o arquivo alheio e para o inexistente. Cada tentativa vira um evento `authz.denied` no log, sem o token|
|TS03|Requisição sem token, com token malformado, assinado por outra chave, expirado, sem assinatura (`alg: none`) ou com o usuário trocado no payload|401|
|TS04|A URL é usada com 4min59s e com 5min01s|200 e depois 403|
|TS05|A URL é usada sem assinatura, com a assinatura alterada, com a validade estendida e no lugar de outro arquivo|403, sem nenhum conteúdo|

O TS01 é o caso válido, e os demais são casos maliciosos, inválidos ou não autorizados. O código está em [practice1_authorization_test.go](../codigo/etapa-4/tests/practice1_authorization_test.go).

### Implementação

A identidade, a autorização e a entrega do arquivo ficam em pontos separados, e cada um nega por padrão:

|Peça|Arquivo|O que garante|
|-|-|-|
|Autenticação|[auth.go]|O `RequireUser` lê o token Bearer. O `Verify` aceita só EdDSA, exige `exp` e confere `sub` e `role`. O `User` só nasce de um token válido, e nenhum handler lê a identidade da requisição|
|Autorização central|[authz.go]|O `CanReadFile` libera só o dono e nega todo o resto|
|Repositório|[files.go]|O `FindForUser` é a única leitura de arquivo e sempre passa pelo `authz`|
|Download|[files.go]|Gera a URL pré-assinada de 5 minutos ou responde 403 e grava o evento `authz.denied`|
|Armazenamento|[storage.go]|Assina a URL com HMAC-SHA256 da chave e da validade, e confere a assinatura em tempo constante (`hmac.Equal`)|
|Auditoria|[audit.go]|Grava uma linha JSON por evento, com horário UTC e origem, sem o token|

O centro da D01 é a única forma de ler um arquivo:

```go
func (r *MemoryRepo) FindForUser(_ context.Context, u auth.User, id FileID) (File, error) {
	r.mu.RLock()
	f, ok := r.files[id]
	r.mu.RUnlock()
	if !ok || authz.CanReadFile(u, f.Owner) != nil {
		return File{}, ErrNotFound
	}
	return f, nil
}
```

Como não existe busca só por ID, uma rota nova não tem como esquecer a autorização. O arquivo inexistente e o alheio devolvem o mesmo `ErrNotFound`, o que impede descobrir quais IDs existem.

### Resultado esperado

Os testes TS01 a TS05 passam:

* o dono recebe a URL, que entrega o arquivo por 5 minutos e depois expira;
* o cliente que pede o arquivo de outro recebe 403, mesmo com `?user_id=`, e a resposta é igual à de um arquivo inexistente. A tentativa fica no log, sem o token;
* sem token válido, a resposta é 401, inclusive com `alg: none`, com outra chave ou com o `sub` trocado;
* a URL alterada recebe 403 do armazenamento, sem nenhum conteúdo.

### Referência da OWASP

* [Authorization Cheat Sheet][cs-authz]: negar por padrão e conferir a permissão em toda requisição.
* [IDOR Prevention Cheat Sheet][cs-idor]: conferir o dono de cada objeto, com IDs aleatórios só como camada extra.
* [ASVS 5.0.0, requisito 8.2.2][asvs-v8]: acesso a cada objeto só com permissão explícita.
* [ASVS 5.0.0, requisitos 9.1.1 e 9.1.2][asvs-v9]: conferir a assinatura do token antes de usar o conteúdo e aceitar só algoritmos de uma lista, sem o `none`.
* [Logging Cheat Sheet][cs-logging]: não gravar tokens de acesso no log.

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

O TS06 é o caso válido, e os demais são casos maliciosos ou inválidos. O código está em [practice2_upload_test.go](../codigo/etapa-4/tests/practice2_upload_test.go).

### Implementação

A validação acontece inteira antes de qualquer gravação:

|Peça|Arquivo|O que garante|
|-|-|-|
|Limite de tamanho|[files.go]|Um `Content-Length` acima de 100 MiB é recusado sem ler o corpo. Sem `Content-Length`, o `http.MaxBytesReader` corta a leitura no limite|
|Estrutura do STL|[stl.go]|O `Parse` confere `84 + 50 * n` e só depois o limite de triângulos. Os erros são tipados (`FormatError` e `TooManyTrianglesError`) e levam o tamanho e o número de triângulos para o log|
|Gravação|[files.go]|O arquivo só é gravado depois da validação, com o hash SHA-256 ([C16])|
|Tarefa|[queue.go]|A fila só aceita um `stl.Validated`, e só o `Parse` cria um. Um arquivo que não passou pela validação nem compila como tarefa|

A ordem das conferências é o que separa os dois erros de conteúdo:

```go
func Parse(data []byte) (Validated, error) {
	if len(data) < headerSize {
		return Validated{}, &FormatError{Size: len(data)}
	}
	n := binary.LittleEndian.Uint32(data[80:headerSize])
	if int64(len(data)) != expectedSize(n) {
		return Validated{}, &FormatError{Size: len(data), Triangles: n}
	}
	if n > MaxTriangles {
		return Validated{}, &TooManyTrianglesError{Triangles: n}
	}
	return Validated{triangles: n}, nil
}
```

Um PDF ou um STL em texto tem um número qualquer no lugar dos triângulos, então é barrado pelo tamanho (`invalid_stl`). Só um STL íntegro chega à regra dos triângulos (`too_many_triangles`).

### Resultado esperado

Os testes TS06 a TS09 passam:

* o STL íntegro é gravado e vira uma tarefa com o dono, o hash e o número de triângulos, e a resposta é 202 com o estado `processing`;
* um arquivo acima de 100 MiB recebe 413 antes de qualquer gravação, declare ou não o tamanho;
* PDF, STL em texto, truncado ou com bytes sobrando recebem 422 `invalid_stl`;
* um STL íntegro com 2.000.001 triângulos recebe 422 `too_many_triangles`;
* em todos os casos recusados, nada vai para o armazenamento, para o repositório nem para a fila.

### Referência da OWASP

* [File Upload Cheat Sheet][cs-upload]: validar o conteúdo, e não a extensão nem o `Content-Type`, e limitar o tamanho do upload.
* [ASVS 5.0.0, requisitos 5.2.1 e 5.2.2][asvs-v5]: aceitar só arquivos que possam ser processados sem negação de serviço e conferir se o conteúdo corresponde à extensão.
* [API4:2023 Unrestricted Resource Consumption][API4:2023]: tamanho máximo de upload antes do processamento caro.

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
[auth.go]: ../codigo/etapa-4/internal/auth/auth.go
[authz.go]: ../codigo/etapa-4/internal/authz/authz.go
[files.go]: ../codigo/etapa-4/internal/files/files.go
[storage.go]: ../codigo/etapa-4/internal/storage/storage.go
[audit.go]: ../codigo/etapa-4/internal/audit/audit.go
[stl.go]: ../codigo/etapa-4/internal/stl/stl.go
[queue.go]: ../codigo/etapa-4/internal/queue/queue.go
[C16]: etapa-2-riscos-nist.md#c16
[cs-upload]: https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html
[asvs-v5]: https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x14-V5-File-Handling.md
[API4:2023]: https://api-security.owasp.org/editions/2023/en/0xa4-unrestricted-resource-consumption/
[cs-authz]: https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html
[cs-idor]: https://cheatsheetseries.owasp.org/cheatsheets/Insecure_Direct_Object_Reference_Prevention_Cheat_Sheet.html
[cs-logging]: https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html
[asvs-v8]: https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md
[asvs-v9]: https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x18-V9-Self-contained-Tokens.md
[RFC 9457]: https://www.rfc-editor.org/rfc/rfc9457
[cs-errors]: https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html
