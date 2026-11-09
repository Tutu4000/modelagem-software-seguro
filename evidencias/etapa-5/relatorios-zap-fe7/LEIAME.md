# ZAP depois da FE7

Rodada do OWASP ZAP com o front-end completo (FE0 a FE7), feita pelo `just zap out=relatorios-zap-fe7`. A configuração das duas varreduras é a da rodada da FE0:

- **`zap-relatorio.*`:** a API, guiada pelo `openapi.yaml` e autenticada como `customer-a`. Desde a FE0, a superfície ganhou os chamados e a lista de técnicos (no painel, fora desta varredura).
- **`zap-front-relatorio.*`:** a varredura *baseline* do app do cliente (`https://nginx:8443/`), com spider e regras passivas.

O painel não entra: ele só escuta no loopback do host (o papel da VPN), fora da rede em que o ZAP roda. Os controles dele são provados pelo Playwright (`web/e2e/abuse.spec.ts`, `panel-orders.spec.ts` e `panel-admin.spec.ts`) e pelo e2e em Go.

As rodadas anteriores estão em [../relatorios-zap/](../relatorios-zap/) (Etapa 5) e em [../relatorios-zap-fe0/](../relatorios-zap-fe0/) (FE0), sem alteração.

## Resultado

Nenhuma falha (FAIL) nas duas varreduras: 113 regras aprovadas na API e 61 no front.

| Alerta | Risco | FE0 | FE7 | Conclusão |
|-|-|-|-|-|
| SQL Injection (40018) | Alto | 5 | 2 | **Falso positivo**, o mesmo da FE0 ([explicação](../relatorios-zap-fe0/LEIAME.md#sql-injection-por-que-é-falso-positivo)). Desta vez os dois casos são `subject` de `POST /api/v1/tickets` e `event_id` do webhook de pagamento, e as duas respostas comparadas pelo ZAP são 429: a varredura estourou o limite de 60 requisições por minuto, e a diferença entre as respostas vem dele, não do SQL |
| Unexpected Content-Type was returned (100001) | Baixo | 44 | 46 | Os erros saem como `application/problem+json` (RFC 9457), como o `openapi.yaml` declara. Vários são 429 da própria varredura |
| Insufficient Site Isolation Against Spectre (90004) | Baixo | — | 3 | **Risco aceito.** Ver abaixo |
| ZAP is Out of Date (10116) | Baixo | — | 1 | Sobre a imagem do ZAP, fixada por digest no compose. Não é do sistema |
| A Client Error response code was returned (100000) | Informativo | 64 | 64 | As recusas esperadas: 400, 401, 403, 404, 409, 422 e 429 |
| Non-Storable / Storable Content (10049) | Informativo | — | 19 | A API responde sem cache compartilhado; os assets com hash ficam em cache longo e o `index.html` vai com `no-cache` |
| Re-examine Cache-control Directives (10015) | Informativo | 5 | 5 | O mesmo da FE0 |
| Information Disclosure - Suspicious Comments (10027) | Informativo | 1 | 2 | Comentários de bibliotecas no bundle minificado (three.js e oidc-client-ts). Nada de segredo ou rota interna |
| Modern Web Application (10109) | Informativo | 3 | 3 | Só avisa que o site é um SPA |

## Isolamento contra Spectre (90004)

O ZAP pede `Cross-Origin-Embedder-Policy` nas páginas do app. Os apps já mandam `Cross-Origin-Opener-Policy: same-origin` (o painel, `same-origin-allow-popups`, por causa do step-up em popup) e `Cross-Origin-Resource-Policy`. Faltaria o COEP para o navegador isolar o processo por completo. Ele não foi ligado:

- **O que o COEP protege não existe nos apps.** O isolamento completo libera `SharedArrayBuffer` e os timers de alta resolução, as ferramentas de um ataque Spectre dentro da página. Os apps não usam nenhum dos dois, e sem o isolamento o navegador continua sem oferecê-los.
- **O `require-corp` quebraria o login.** A renovação silenciosa da sessão roda num iframe que passa pelo Keycloak, em outra origem (`https://localhost:8444`), e o Keycloak não manda `Cross-Origin-Resource-Policy` nem COEP. O `credentialless` não resolve, porque o iframe precisa do cookie de sessão do Keycloak.
- **O que já protege:** a CSP sem script de terceiros, o COOP e o CORP em todas as respostas, e os tokens só em memória.

Ligar o COEP pede que o Keycloak sirva os iframes com CORP e COEP, ou que a renovação deixe de usar iframe. Fica no backlog.

## O que a FE7 acrescenta à varredura

Os testes de abuso do Playwright cobrem o que uma varredura passiva não vê:
- XSS armazenado contra a equipe no painel;
- token sem MFA e de cliente na API do painel;
- IDs inventados e caminhos que sobem acima da raiz;
- operação crítica sem justificativa.

Também cobrem o que já vinha das fases anteriores: pedido e chamado de outro cliente, e o técnico que não é o atribuído.
