# ZAP depois da FE0

Esta é a nova rodada do OWASP ZAP, feita depois que a FE0 ligou os cabeçalhos de segurança no nginx e passou a servir os apps web. Ela tem duas varreduras:

- **`zap-relatorio.*`:** a API, guiada pelo `openapi.yaml` e autenticada como `customer-a`, igual à da Etapa 5. A API agora tem mais rotas (pedidos, pagamento, webhooks, chamados, troca de endereço), então a superfície é maior que a da primeira rodada.
- **`zap-front-relatorio.*`:** a varredura *baseline* do app do cliente (`https://nginx:8443/`), com spider e regras passivas.

A rodada anterior está em [../relatorios-zap/](../relatorios-zap/) e não foi alterada.

## O que mudou na API

| Alerta da Etapa 5 | Risco | Agora | Por quê |
|-|-|-|-|
| Strict-Transport-Security Header Not Set (10035) | Baixo | **Sumiu** | HSTS em todas as respostas (`max-age=31536000`) |
| Server Leaks Version Information (10036) | Baixo | **Sumiu** | `server_tokens off` no nginx |
| ZAP is Out of Date (10116) | Baixo | Sumiu | É sobre a imagem do ZAP, não sobre o sistema |
| Non-Storable Content (10049) | Informativo | Sumiu | Informativo |
| Unexpected Content-Type was returned (100001) | Baixo | Continua | Ver abaixo |
| A Client Error response code was returned (100000) | Informativo | Continua | Ver abaixo |

## Alertas desta rodada

| Alerta | Risco | Onde | Conclusão |
|-|-|-|-|
| SQL Injection (40018) | Alto | `POST /api/v1/orders` (`city`, `color`, `file_id`) e os dois webhooks (`event_id`, `shipment_id`) | **Falso positivo.** Ver a seção abaixo |
| Unexpected Content-Type was returned (100001) | Baixo | Respostas de erro da API | Os erros saem como `application/problem+json` (RFC 9457), que o `openapi.yaml` declara. O ZAP esperava `application/json`. Não há o que corrigir |
| A Client Error response code was returned (100000) | Informativo | Várias rotas | São as recusas esperadas: 400, 401, 403, 404, 409 e 422 para entradas inválidas, recursos de outros clientes e assinaturas forjadas |
| Re-examine Cache-control Directives (10015) | Informativo | API e `index.html` | O `index.html` vai com `Cache-Control: no-cache`, para o app novo valer na hora, e os assets com hash ficam em cache longo. As respostas da API não têm dado em cache compartilhado |
| Information Disclosure - Suspicious Comments (10027) | Informativo | Bundle JavaScript do app | Comentários de bibliotecas no código minificado. Não há segredo nem rota interna |
| Modern Web Application (10109) | Informativo | App do cliente | Só avisa que o site é um SPA |

## SQL Injection: por que é falso positivo

Nos relatórios, o ZAP mandou o mesmo campo com duas condições, como `East Romaineburgh AND 1=1 -- ` e `East Romaineburgh AND 1=2 -- `, viu respostas diferentes e concluiu que o SQL foi alterado. Três fatos mostram que não foi:

1. **Toda consulta é parametrizada.** O acesso ao banco é gerado pelo `sqlc` a partir de `queries.sql`, e os valores vão como parâmetros (`$1`, `$2`, ...), nunca concatenados. A única concatenação de SQL no código Go é a contagem do backup (`internal/backup/backup.go`), feita sobre nomes de tabela constantes.
2. **A reprodução manual dá respostas iguais.** O mesmo orçamento, mandado com `city` igual a `East Romaineburgh AND 1=1 -- ` e depois a `East Romaineburgh AND 1=2 -- `, deu nas duas vezes `201`, o mesmo `total_cents` e a cidade gravada literalmente, com o texto do ataque como texto.
3. **A diferença vista pelo ZAP vem de outra coisa:**
   - os webhooks recusam toda notificação sem assinatura válida com o mesmo `400 invalid_signature`;
   - o orçamento responde de forma diferente conforme o arquivo, a cota e o limite de 60 requisições por minuto, que a varredura estoura (429);
   - cada orçamento criado tem ID e horário próprios.

## Front-end

A baseline do app do cliente terminou sem falhas: 0 FAIL, 3 WARN, todos informativos, e 55 regras aprovadas. Entre as aprovadas estão as que a Etapa 5 não conseguia cobrir, porque ainda não havia página:

- CSP ausente ou fraca;
- anti-clickjacking (`frame-ancestors 'none'`);
- `X-Content-Type-Options`;
- cookies;
- HSTS.
