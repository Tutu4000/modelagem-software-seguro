# Etapa 5 — Verificação de Vulnerabilidades

Esta etapa verifica o sistema construído na [Etapa 4](etapa-4-codigo-seguro.md) com o OWASP ZAP e analisa cinco achados: os três pedidos pelo enunciado e dois extras. Dois vieram dos alertas do ZAP, dois de testes manuais extensivos pela interface, depois confirmados no código, que alcançam falhas fora do alcance da ferramenta, e um de uma varredura dirigida, em que o ZAP provocou um erro que só os logs do servidor mostraram. Os relatórios, as capturas de tela e as evidências brutas estão em [evidencias/etapa-5/](../evidencias/etapa-5/), descritos no [relatório da verificação](../evidencias/etapa-5/relatorio-da-verificacao.md).

## 1. Sistema testado

O alvo é o nosso próprio sistema, a plataforma de impressão 3D sob demanda, rodando no ambiente local do Docker Compose ([codigo/etapa-4/](../codigo/etapa-4/)). Nenhum sistema de terceiros foi testado.

|Parte|Como foi verificada|
|-|-|
|API do cliente|ZAP, com varredura ativa|
|App web do cliente|ZAP, com spider e regras passivas|
|Painel interno|ZAP, só na rodada 4, com um token de cada papel (admin, técnico e suporte). Nas rodadas automáticas ele ficou de fora, porque só escuta no loopback do host|
|Aplicação inteira|Testes manuais extensivos pela interface, no app do cliente e no painel, com cada comportamento estranho confirmado no código|
|Código Go e imagens|`gosec`, `govulncheck`, `gitleaks` e Trivy|

## 2. Ferramenta e configuração

* **Ferramenta:** OWASP ZAP 2.16.1, na imagem oficial `zaproxy/zap-stable`, fixada por digest no [compose.yaml](../codigo/etapa-4/deploy/compose.yaml).
* **Execução:** `just zap`, em `codigo/etapa-4/`. O ZAP roda como contêiner na rede pública do compose e acessa o sistema pelo nginx, como um cliente externo.
* **API:** `zap-api-scan.py`, guiado pelo [openapi.yaml](../codigo/etapa-4/openapi.yaml). As requisições levam o token de um cliente de teste (`customer-a`), obtido no Keycloak antes da varredura.
* **App do cliente:** varredura *baseline*, com spider de 1 minuto e regras passivas. O plano está em [zap.yaml](../evidencias/etapa-5/relatorios-zap-fe7/zap.yaml).
* **Varredura dirigida (rodada 4):** o ZAP roda como serviço e é controlado pela API dele. Primeiro recebe pelo proxy requisições reais de cada rota, com IDs e corpos válidos, e depois ataca essas rotas. O token é renovado a cada 3 minutos, e os limites de requisição ficam folgados só no ambiente local. O método está no [LEIAME da rodada](../evidencias/etapa-5/relatorios-zap-dirigido/LEIAME.md).

## 3. Evidência da execução

A varredura foi repetida a cada fase do sistema. As três primeiras rodadas são automáticas, e a quarta é dirigida.

|Rodada|Data|Alvo|Relatórios|Captura|
|-|-|-|-|-|
|1|07/11/2026|API|[relatorios-zap/](../evidencias/etapa-5/relatorios-zap/)|[API][cap-1]|
|2|08/11/2026|API e app do cliente|[relatorios-zap-fe0/](../evidencias/etapa-5/relatorios-zap-fe0/)|—|
|3|09/11/2026|API e app do cliente, com o sistema completo|[relatorios-zap-fe7/](../evidencias/etapa-5/relatorios-zap-fe7/)|[API][cap-3], [app][cap-3-app]|
|4|14/11/2026|API e painel interno, em varredura dirigida|[relatorios-zap-dirigido/](../evidencias/etapa-5/relatorios-zap-dirigido/)|[API e painel][cap-4]|

Na rodada 3, o ZAP aprovou 113 regras na API e 61 no app, sem nenhuma reprovação. Mas 37 das 64 respostas de erro que ele registrou eram `429`: o limite de 60 requisições por minuto barrou boa parte dos ataques antes do código. Na rodada 4, com os limites folgados, o ZAP fez 7.490 requisições à API e 30.176 ao painel, e só 46 receberam `429`.

![Resumo do relatório do ZAP na rodada 3, com um alerta alto de SQL Injection][cap-3]

## 4. Achados analisados

|ID|Alerta ou achado|Origem|Evidência|Possível impacto|Relação com OWASP ou CWE|Correção proposta|
|-|-|-|-|-|-|-|
|A01|Cabeçalho `Strict-Transport-Security` ausente e versão do servidor exposta (`Server: nginx/1.30.6`)|ZAP, rodada 1|3 ocorrências de cada alerta no [relatório da rodada 1][cap-1]|Sem HSTS, o navegador aceita voltar para HTTP numa rede hostil. A versão exposta ajuda o atacante a escolher falhas conhecidas|[CWE-319], [CWE-497] e [A02:2025] Security Misconfiguration|**Aplicada:** HSTS em todas as respostas e `server_tokens off` no nginx. Os dois alertas sumiram na rodada 2|
|A02|SQL Injection em `subject` (chamados) e `event_id` (webhook de pagamento)|ZAP, rodada 3|2 ocorrências de risco alto no [relatório da rodada 3][cap-3]|Se fosse real, leitura ou alteração do banco inteiro|[CWE-89] e [A05:2025] Injection|**Falso positivo.** Nenhuma correção: o código já usa consultas parametrizadas|
|A03|Estouro de inteiro no cálculo do preço, a partir do volume do STL|Testes manuais, confirmados no código|Teste que devolve um total de R$ 10,00 para um volume de 4,6 × 10²¹ mm³, e três respostas 500 no teste pela interface|Uma peça de volume absurdo sai pelo preço mínimo, ou o cliente provoca erros internos quando quiser|[CWE-190] e [CWE-681]; [A10:2025] Mishandling of Exceptional Conditions; risco [R07]|**Aplicada:** o orçamento recusa o volume antes da conversão para inteiro, com 400|
|A04 (extra)|Negação de serviço pelo upload: o corpo inteiro ia para a memória antes da validação|Testes manuais, confirmados no código|Teste em que a API lê 52.428.884 bytes de um corpo recusável pelos primeiros 84, e aceita um terceiro upload de 100 MB com dois presos|De 4 a 6 uploads paralelos de 100 MB estouram os 512 MB do contêiner e derrubam a API para todos|[CWE-770] e [CWE-400]; [API4:2023] Unrestricted Resource Consumption; riscos [R19] e [R20], requisito [RS03]|**Aplicada:** leitura limitada ao tamanho que o cabeçalho declara e um orçamento de 256 MiB para todos os uploads juntos|
|A05 (extra)|Byte nulo no endereço de entrega provoca erro interno|ZAP dirigido, rodada 4, e log do servidor|8 respostas `500` em `POST /api/v1/orders` e 8 em `POST /panel/v1/orders/{id}/address-change`, com o erro `invalid byte sequence for encoding "UTF8": 0x00`|Qualquer cliente provoca erros internos quando quiser, o que suja os logs e dispara o alerta de 5xx. Quebras de linha no endereço seguiriam para a transportadora e para o e-mail|[CWE-20] e [CWE-158]; [A10:2025] Mishandling of Exceptional Conditions|**Aplicada:** o endereço recusa caracteres de controle, com 400|

### A01 — Cabeçalhos de segurança ausentes

A rodada 1 foi feita só com a API, antes do front-end. O ZAP apontou que as respostas não traziam `Strict-Transport-Security` e que o cabeçalho `Server` mostrava `nginx/1.30.6`. São problemas de configuração, e não de código. A correção ficou no nginx: `add_header Strict-Transport-Security "max-age=31536000" always` e `server_tokens off`. A rodada 2 confirma a correção, porque os dois alertas não aparecem mais.

### A02 — SQL Injection, um falso positivo

O ZAP enviou o mesmo campo com duas condições, `AND 1=1 --` e `AND 1=2 --`, recebeu respostas diferentes e concluiu que o SQL tinha sido alterado. Três fatos mostram que não foi:

1. **Toda consulta é parametrizada.** O acesso ao banco é gerado pelo `sqlc`, e os valores vão como parâmetros, nunca concatenados.
2. **A reprodução manual dá respostas iguais.** O mesmo pedido, com as duas condições, devolveu o mesmo status e gravou o texto do ataque como texto.
3. **A diferença vinha de outro lugar.** As respostas comparadas eram `429`: a varredura estourou o limite de 60 requisições por minuto. No webhook, toda notificação sem assinatura válida recebe o mesmo `400`.

O alerta é o de maior risco do relatório, e mesmo assim não é uma falha. Aceitá-lo sem análise levaria a "corrigir" um código que já está certo.

A rodada 4 confirma a análise. Com os limites de requisição folgados, o ZAP repetiu os mesmos ataques de SQL Injection nas mesmas rotas, não recebeu `429` e **não gerou o alerta**.

### A03 — Estouro de inteiro no preço

O preço de uma peça vem do volume do STL, calculado pelo worker. Um arquivo bem formado, mas com um vértice a 2,36 × 10²¹ mm de distância, tem um volume de 2,14 × 10²³ mm³. No cálculo original, esse valor era convertido para um inteiro de 64 bits sem checagem:

* o preço unitário virava −9.223.372.036.854.775.808 centavos, o menor inteiro possível;
* multiplicado por uma quantidade par, o resultado dava a volta e zerava, e o total ficava só com a taxa de setup: **R$ 10,00**.

No teste pela interface, com o código original, o pedido não chegou a ser criado. A transportadora simulada também estourou, devolveu um frete negativo, e a restrição `orders_shipping_cents_check` do banco recusou a gravação. A API respondeu `500` três vezes. Ou seja, a falha passou pela aplicação e foi contida por acaso, três camadas adiante: com uma transportadora que cobrasse um teto fixo, o pedido teria sido gravado com o preço da peça zerado.

A correção recusa o volume no próprio cálculo, quando o preço unitário passaria de 1 bilhão de reais, e a API responde `400` sem chamar a transportadora nem o banco. O worker também passou a recusar coordenadas infinitas ou NaN e volume maior que a caixa envolvente.

### A04 — Negação de serviço pelo upload

O requisito [RS03] manda conferir o arquivo antes de gravar. O código conferia, mas só depois de ler o corpo inteiro para a memória, até 100 MiB por requisição. O limite por usuário é de 60 requisições por minuto, e nada limitava os uploads simultâneos. Um único cliente autenticado, com poucos uploads paralelos de lixo, ocuparia mais memória do que os 512 MB do contêiner da API.

A correção lê primeiro os 84 bytes do cabeçalho do STL, que declaram o número de triângulos e, com ele, o tamanho exato do arquivo:

* um `Content-Length` diferente desse tamanho é recusado sem ler o corpo;
* a memória reservada é a do tamanho declarado, e o que sobra é descartado sem ser guardado;
* todos os uploads juntos dividem um orçamento de 256 MiB, e o que não cabe recebe `503`.

**Limitação desta evidência:** o teste automatizado prova que não havia limite e que o corpo era lido inteiro. Não derrubamos o contêiner de verdade com o código original.

### A05 — Byte nulo no endereço

Na rodada 4, o ZAP enviou um caractere nulo (`\u0000`) nos campos de texto. Os chamados e o cancelamento recusaram com `400`. O endereço de entrega não: a validação conferia só o tamanho de `recipient`, `street`, `number` e `city`. O valor chegou ao PostgreSQL, que não aceita o byte nulo em texto, e a API respondeu `500`. O mesmo aconteceu no painel, na troca de endereço proposta pelo suporte, porque as duas rotas usam a mesma validação.

O ZAP não gerou nenhum alerta para isso. Foram as requisições dele que causaram o erro, mas o relatório não marca uma resposta `500`. O achado apareceu ao conferir os logs da API e do painel depois da varredura, e foi reproduzido à mão nos quatro campos.

A correção recusa caracteres de controle no endereço, o que cobre também as quebras de linha. O teste novo falhava nos seis casos antes dela.

## 5. Alertas descartados

|Alerta do ZAP|Risco|Ocorrências na rodada 3|Por que foi descartado|
|-|-|-|-|
|Unexpected Content-Type was returned|Baixo|46|Os erros saem como `application/problem+json` (RFC 9457), que o `openapi.yaml` declara. O ZAP esperava `application/json`|
|Insufficient Site Isolation Against Spectre|Baixo|3|Risco aceito. O cabeçalho que falta (COEP) quebraria a renovação de sessão pelo Keycloak, e os apps não usam os recursos que ele protege|
|ZAP is Out of Date|Baixo|2|É sobre a imagem do ZAP, e não sobre o sistema|
|A Client Error response code was returned|Informativo|64|São as recusas esperadas: 400, 401, 403, 404, 409, 422 e 429|
|Cache-control, conteúdo armazenável, comentários suspeitos e Modern Web Application|Informativo|29|Informativos. Os comentários são de bibliotecas no código minificado, sem segredo nem rota interna|

Um alerta da rodada 4 não foi descartado: **CSP: Failure to Define Directive with No Fallback** (risco médio, 1 ocorrência). A CSP das respostas da API não definia `form-action`, que não herda de `default-src`. O impacto é baixo, porque a API devolve JSON e não páginas, mas a correção é pequena e foi aplicada no commit [2472ce4]: `form-action 'none'; base-uri 'none'` nas respostas da API, do painel e do armazenamento.

A análise completa de cada rodada está nos arquivos `LEIAME.md` das pastas de relatórios.

## 6. O que o ZAP não alcança

O ZAP encontrou o que aparece na resposta HTTP: cabeçalhos, versão do servidor e formato dos erros. O único alerta alto dele era falso. As falhas mais graves desta etapa apareceram nos testes manuais e foram confirmadas no código, e nenhuma delas é visível para uma varredura:

|Achado dos testes manuais|Por que o ZAP não vê|Situação|
|-|-|-|
|Estouro de inteiro no preço (A03)|Exige montar um STL binário com coordenadas escolhidas, e o ZAP não conhece o formato|Corrigido|
|Negação de serviço pelo upload (A04)|O ZAP envia uma requisição por vez e não mede a memória do servidor|Corrigido|
|Malha inválida aceita pelo worker|O processamento é assíncrono, fora da resposta HTTP|Corrigido|
|Logout que deixava a sessão válida depois de uma falha do Valkey|Depende de uma falha de infraestrutura no meio da operação|Corrigido|
|Token renovado durante a revogação de um usuário continuava aceito|É uma condição de corrida entre a API e o Keycloak|Corrigido|
|Técnico abria o projeto de um pedido já encerrado|É regra de negócio: para o scanner, a resposta `200` com a URL do arquivo é um sucesso normal|Corrigido|
|10 CVEs altas ou críticas na imagem do nginx|O ZAP olha o tráfego, e não os pacotes instalados. Quem achou foi o Trivy|Corrigido|
|Backup que quebra com cerca de 200 MB de arquivos, falta de cota de armazenamento por conta e corrida entre pagar e cancelar|Dependem de volume de dados e de concorrência|Pendentes|
|Peça com volume quase zero orçada pelo preço mínimo|É uma fraude de regra de negócio, indistinguível de uma peça oca legítima. A defesa hoje é a revisão técnica obrigatória|Pendente|

Cada correção tem um teste que falha no código anterior e passa com ela. Os testes estão no commit [75ff909], e as correções, nos commits [a0cd7e8] e [7832333]. O teste e a correção do A05 vieram depois, com a rodada 4.

## 7. Considerações finais

* **A ferramenta e os testes manuais se completam.** O ZAP é rápido e repetível e pegou dois problemas reais de configuração. Ele não entende regra de negócio, formato de arquivo, concorrência nem consumo de recursos.
* **Um alerta de risco alto não é uma falha confirmada.** O SQL Injection do relatório era efeito do limite de requisições da própria API.
* **Defesa em profundidade funcionou, mas por acaso.** No A03, a restrição do banco segurou o que a aplicação deixou passar. A correção põe a checagem no lugar certo.
* **A varredura só é boa se alcançar o código.** Nas rodadas automáticas, o limite de requisições barrou mais da metade dos ataques. A rodada dirigida, com token renovado e limites folgados, achou o A05 em 17 minutos.
* **O relatório da ferramenta não basta.** O A05 não gerou alerta: ele só apareceu nos logs do servidor.
* **Limitações desta verificação:** o A04 foi demonstrado por teste e não por um ataque real, a rodada 4 não varreu o app web nem os webhooks além da checagem de assinatura, e os achados pendentes da seção 6 continuam abertos.

[cap-1]: ../evidencias/etapa-5/capturas-de-tela/zap-rodada-1-api.png
[cap-3]: ../evidencias/etapa-5/capturas-de-tela/zap-rodada-3-api.png
[cap-3-app]: ../evidencias/etapa-5/capturas-de-tela/zap-rodada-3-app-cliente.png
[cap-4]: ../evidencias/etapa-5/capturas-de-tela/zap-rodada-4-dirigida.png

[R07]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R19]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R20]: etapa-2-riscos-nist.md#22-registro-de-riscos
[RS03]: etapa-3-arquitetura-segura.md#1-requisitos-de-segurança

[CWE-20]: https://cwe.mitre.org/data/definitions/20.html
[CWE-89]: https://cwe.mitre.org/data/definitions/89.html
[CWE-158]: https://cwe.mitre.org/data/definitions/158.html
[CWE-190]: https://cwe.mitre.org/data/definitions/190.html
[CWE-319]: https://cwe.mitre.org/data/definitions/319.html
[CWE-400]: https://cwe.mitre.org/data/definitions/400.html
[CWE-497]: https://cwe.mitre.org/data/definitions/497.html
[CWE-681]: https://cwe.mitre.org/data/definitions/681.html
[CWE-770]: https://cwe.mitre.org/data/definitions/770.html
[A02:2025]: https://top10.owasp.org/2025/A02_2025-Security_Misconfiguration/
[A05:2025]: https://top10.owasp.org/2025/A05_2025-Injection/
[A10:2025]: https://top10.owasp.org/2025/A10_2025-Mishandling_of_Exceptional_Conditions/
[API4:2023]: https://api-security.owasp.org/editions/2023/en/0xa4-unrestricted-resource-consumption/

[75ff909]: https://github.com/Tutu4000/modelagem-software-seguro/commit/75ff909f1d8b7c9b51ec78d5c25b6958a9099719
[a0cd7e8]: https://github.com/Tutu4000/modelagem-software-seguro/commit/a0cd7e872446a33ba4f6368f2df0788ddd4fa821
[7832333]: https://github.com/Tutu4000/modelagem-software-seguro/commit/783233325f087d9f766c3d261cf1ca024205d583
[2472ce4]: https://github.com/Tutu4000/modelagem-software-seguro/commit/2472ce4270eee6572fd6a76c2206f60a8816bb4b
