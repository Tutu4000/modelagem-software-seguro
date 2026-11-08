# Front-end: plano macro

Plano das fases do front-end, no mesmo formato do [BACKLOG.md](BACKLOG.md):

- cada fase tem um plano próprio, detalhado quando começa;
- cada fase tem branch, testes, commit e PR contra a anterior.

O item do backlog é o E8.6, aplicação web e visualizador 3D com CSP (C09).

## 1. O que o back-end já entrega

- **Clientes do Keycloak:** já existem dois clientes públicos com PKCE:

  | Cliente | Endereço | Exigência |
  |-|-|-|
  | `web` | `https://localhost/*` | — |
  | `panel-web` | `https://localhost:9445/*` | MFA (`acr=2`) |

- **Mesma origem que a API, sem CORS:**

  | App | Servido em | API |
  |-|-|-|
  | Cliente | `/` da porta 443 | `/api/` |
  | Painel | `/` da porta 9445 | `/panel/` |

- **Contratos:** [openapi.yaml](openapi.yaml) e [panel-openapi.yaml](panel-openapi.yaml) completos, com os erros em Problem Details (RFC 9457).
- **Sem Node em execução:** o front vira arquivos estáticos, servidos pelo nginx. O Node só entra no build, dentro do Docker.

## 2. Decisões

### Stack

- **React com TypeScript em modo estrito, sem `any`:**
  - no `tsconfig`, `strict`, `noUncheckedIndexedAccess` e `exactOptionalPropertyTypes`;
  - no ESLint, `@typescript-eslint/no-explicit-any` e as regras `no-unsafe-*` como erro.
- **Vite** no build e **React Router** nas rotas.
- **TanStack Query (React Query)** para os dados, o cache e o polling do processamento e dos webhooks.
- **Tipos da API gerados dos dois OpenAPI** (`openapi-typescript`), incluindo as respostas de erro.
- **Dois apps separados, cliente e painel,** e um pacote comum. O painel não compartilha bundle nem origem com o app do cliente, assim como o binário `panel` não está na API.
- **Tokens só em memória,** nunca em `localStorage` ou `sessionStorage`. Ao recarregar a página, a sessão volta pelo SSO do Keycloak (`prompt=none`).
- **Testes:**
  - unidade com Vitest e Testing Library;
  - ponta a ponta com Playwright contra o compose, usando as contas do `devlogin` e o `just seed`.

### Segurança, já na FE0

- **Cabeçalhos no nginx, nas portas públicas e no painel:**
  - CSP estrita, sem `unsafe-inline` nem `unsafe-eval`;
  - HSTS;
  - `X-Content-Type-Options: nosniff`;
  - `Referrer-Policy: no-referrer`;
  - `Permissions-Policy`;
  - `Cross-Origin-Opener-Policy` e `Cross-Origin-Resource-Policy`;
  - `frame-ancestors 'none'`;
  - Trusted Types.
- **Nada de recurso de terceiros:** as fontes e os ícones vêm no próprio bundle, então a CSP não precisa liberar CDN nem Google Fonts.
- **ZAP de novo,** contra a API e contra o front, com os relatórios em `evidencias/etapa-5/relatorios-zap-fe0/`. Os da Etapa 5 ficam como estão, para comparação.
- **Texto digitado por usuários** (chamados, laudos, endereços) é sempre renderizado como texto. Uma regra de lint proíbe `dangerouslySetInnerHTML`.

### Direção visual

A referência é o PCBWay, mais moderno: cara de ferramenta industrial de orçamento, com bastante informação e o configurador como centro.

- **Bordas:** retas, raio 0 em tudo (botões, campos, cartões, modais). Separadores de 1 px e sombras quase nulas.
- **Cores:**
  - fundo claro neutro, com texto e cabeçalhos em azul-marinho escuro;
  - laranja de sinal para a ação principal (orçar, pagar);
  - cada estado do pedido tem cor própria, sempre com texto ou ícone junto, para não depender só da cor.
- **Tipografia:** uma sans técnica (Inter ou IBM Plex Sans) e uma mono para IDs, rastreio e valores.
- **Composição:** primitivas pequenas que se combinam, em vez de componentes grandes e configuráveis.

  | Grupo | Primitivas |
  |-|-|
  | Layout | `Stack`, `Grid`, `Panel`, `Toolbar` |
  | Formulário | `Field`, `Select`, `Stepper` |
  | Dados | `DataTable`, `StatusTag`, `Timeline` |

  O configurador de orçamento sai de `Panel` e `Field`, e as telas do painel saem de `Toolbar` e `DataTable`.
- **Responsivo, mobile primeiro:**

  | Tela | Layout |
  |-|-|
  | Celular | Uma coluna. As tabelas viram cartões, e o resumo do orçamento fica fixo embaixo |
  | Tablet | Duas colunas |
  | Desktop | O configurador ao lado do visualizador 3D e do resumo de preço |

- **Visualizador 3D:** ocupa a largura toda no celular, com toque para girar e dar zoom.

## 3. Fases

| Fase | Conteúdo | Referências |
|-|-|-|
| **FE0 Fundação** | Pasta `web/` com os dois apps e o pacote comum: cliente HTTP tipado, tratamento de Problem Details, tokens de design e primitivas. Build num estágio do Dockerfile do nginx. Todos os cabeçalhos de segurança. Lint e `tsc` estritos, Vitest, auditoria de dependências com lockfile fixado, Playwright no compose e ZAP de novo | E8.6, C09, C12 |
| **FE1 Login e sessão** | Authorization code com PKCE (`oidc-client-ts`), tokens em memória, sessão de volta pelo SSO, logout. Tratamento de 401, 429 com `Retry-After` e 503. No painel: `acr=2`, step-up quando a API pede (`WWW-Authenticate` com `max_age`) e a justificativa das operações críticas | E5.3, E6.4, C07, C08 |
| **FE2 Cliente: arquivos e orçamento** | Upload do STL com progresso e o acompanhamento do processamento. Visualizador 3D: three.js com STLLoader, parse num Web Worker e o arquivo lido pela URL pré-assinada. Catálogo e configurador de orçamento, com endereço, frete e alerta de DFM | RF02 a RF07, RF19, RF24, CA01 |
| **FE3 Cliente: do pedido à entrega** | Lista e detalhe dos pedidos, linha do tempo dos estados, laudo e pagamento (cobrança pendente até o webhook). Rastreio, cancelamento com motivo e confirmação da troca de endereço com o código do e-mail | RF08 a RF12, RF20, C21 |
| **FE4 Cliente: suporte** | Chamados e mensagens, sempre como texto | RF11 |
| **FE5 Painel: pedidos** | Login com MFA, listas por papel (admin e suporte, técnico) e detalhe com DFM. Atribuição, download do STL como operação crítica, laudo, produção e coleta | E6.1 a E6.4, CA05, CA08 |
| **FE6 Painel: suporte e administração** | Responder e fechar chamados. Propor troca de endereço, tabelas de preço (propor e aprovar, com dois admins) e revogar usuário, todas como operações críticas | D02, C21 |
| **FE7 Abuso, acessibilidade e documentação** | Playwright com os casos de abuso: payload XSS num chamado, ID alheio na URL e painel sem MFA. Acessibilidade básica (teclado, contraste, rótulos) e documentação no README | C09, CA01, D01, D02 |

## 4. Estado

|Fase|Estado|
|-|-|
|FE0|Concluída: os dois apps servidos pelo nginx, os tipos dos OpenAPI, o design system com a página `/_design` no build de desenvolvimento, todos os cabeçalhos de segurança, o Playwright e o ZAP de novo|
|FE1 a FE7|Pendentes|
