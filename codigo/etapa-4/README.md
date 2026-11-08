# Código: API, worker e ambiente local

Implementação em Go da arquitetura da [Etapa 3](../../docs/etapa-3-arquitetura-segura.md). As duas práticas da Etapa 4 estão documentadas em [docs/etapa-4-codigo-seguro.md](../../docs/etapa-4-codigo-seguro.md). O plano completo, as simplificações e o estado de cada item estão no [BACKLOG.md](BACKLOG.md).

## Pré-requisitos

* Go 1.27
* Docker com Compose v2
* [just](https://github.com/casey/just), que funciona no Linux, no macOS e no Windows

Sem o `just`, cada receita do [justfile](justfile) é um comando de `go` ou `docker compose` que pode ser executado direto.

## Subir o ambiente

Todos os comandos são executados nesta pasta.

```sh
just secrets     # gera chaves, senhas e o certificado TLS em secrets/ (fora do git)
just up          # constrói as imagens e sobe tudo
just scale 3     # opcional: três réplicas da API atrás do nginx
just seed        # STLs e pedidos fictícios dos clientes A e B, do orçamento à produção
```

|Endereço|O que é|
|-|-|
|`https://localhost`|API pública, pelo nginx|
|`https://localhost:8443`|Downloads pelas URLs pré-assinadas do Garage. Só GET e HEAD|
|`https://localhost:8444`|Keycloak, só o realm `nugget`. O console de administração e o realm `master` não são publicados|
|`https://localhost:9445`|Painel interno, publicado só no loopback do host. As rotas do painel não existem nas outras portas|

Numa VM, o painel só é alcançado por túnel SSH, que faz o papel da VPN: `ssh -L 9445:127.0.0.1:9445 <vm>`.

O certificado é de uma CA local. Para o `curl`, use `--cacert secrets/tls_ca.pem`. Para o navegador, importe esse arquivo ou aceite o aviso.

No primeiro `just up`, o Keycloak leva cerca de um minuto para criar o banco e importar o realm. A API só sobe depois que ele fica pronto.

### Contas fictícias

O realm é gerado a partir de [deploy/keycloak/realm.json](deploy/keycloak/realm.json) pelo `just secrets`. Todas as contas usam a senha de `secrets/keycloak_demo_password`. As contas da equipe também pedem um código TOTP, gerado a partir de `secrets/keycloak_totp_secret`; para o navegador, cadastre esse segredo num app autenticador.

|Conta|Perfil|TOTP|
|-|-|-|
|`customer-a`, `customer-b`, `customer-c`, `customer-d`|`customer`|Não|
|`technician`, `technician-2`|`technician`|Sim|
|`support`|`support`|Sim|
|`admin`, `admin-2`|`admin`|Sim|

O `technician-2` nunca recebe pedidos nos testes, para conferir que só o técnico atribuído trabalha num pedido. O `admin-2` aprova as tabelas de preço que o `admin` propõe, porque uma tabela precisa de dois admins.

O `customer-c` fica reservado para o teste de cota do e2e, e o `customer-d` é revogado pelo teste do painel.

O `just token <conta>` faz o login pelo cliente `e2e`, que só existe no realm local e aceita usuário e senha, e imprime o token. Esse token não serve para o painel: sem o fluxo de navegador, o Keycloak não comprova o TOTP. Para o painel, `just panel-token <conta>` faz o login pelo `panel-web`, preenchendo as telas de senha e de TOTP como um navegador. Os front-ends vão usar os clientes `web` e `panel-web`, com authorization code e PKCE. O contrato do painel está em [panel-openapi.yaml](panel-openapi.yaml).

```sh
TOKEN=$(just token a)
curl --cacert secrets/tls_ca.pem -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: model/stl" --data-binary @peca.stl https://localhost/api/v1/files
curl --cacert secrets/tls_ca.pem -H "Authorization: Bearer $TOKEN" https://localhost/api/v1/files/<id>
```

O contrato das rotas está no [openapi.yaml](openapi.yaml).

### Fluxo de um pedido

1. **Cliente:** envia o STL (`POST /api/v1/files`) e espera o estado `done`.
2. **Cliente:** consulta o catálogo (`GET /api/v1/catalog`) e pede o orçamento (`POST /api/v1/orders`), com o endereço de entrega. O preço é calculado só no servidor, e o frete, cotado na transportadora, é somado depois do desconto. O pedido nasce `quoted`, e o orçamento vale 24 horas.
3. **Cliente:** confirma (`POST /api/v1/orders/{id}/submit`), e o pedido passa a `awaiting_review`.
4. **Admin, no painel:** atribui o pedido a um técnico (`POST /panel/v1/orders/{id}/assign`).
5. **Técnico atribuído, no painel:** abre o STL com justificativa (`POST /panel/v1/orders/{id}/file`) e registra o laudo e a decisão (`POST /panel/v1/orders/{id}/review`). Aprovado, o pedido vai para `awaiting_payment`; reprovado, para `rejected`.
6. **Cliente:** paga (`POST /api/v1/orders/{id}/payment`). A resposta é a cobrança pendente; o pedido só passa a `paid` quando chega a notificação assinada do gateway em `POST /api/v1/webhooks/payments`.
7. **Técnico atribuído, no painel:** inicia a impressão (`POST /panel/v1/orders/{id}/production`, para `in_production`) e agenda a coleta (`POST /panel/v1/orders/{id}/ship`). O painel manda o endereço para a transportadora sem mostrá-lo ao técnico e recebe o código de rastreio.
8. **Transportadora:** avisa a coleta e a entrega em `POST /api/v1/webhooks/shipping`, e o pedido passa a `shipped` e depois a `delivered`.
9. **Cliente:** acompanha o pedido, o histórico, a revisão, o pagamento e o rastreio (`GET /api/v1/orders/{id}`).

Os webhooks são rotas públicas, sem token: valem pela assinatura `X-Nugget-Signature: t=<unix>,v1=<HMAC-SHA256 de "t.corpo">`, com um segredo por parceiro, e recusam timestamps com mais de 5 minutos. Um evento repetido responde 200 sem efeito, e um evento cujo valor não bate com o pedido responde 400 e fica registrado como `payment.rejected`. Com um parceiro fora do ar, o orçamento e o pagamento respondem 502 `partner_unavailable`, e o pedido não muda.

Os preços mudam por uma tabela nova, proposta por um admin (`POST /panel/v1/price-tables`) e aprovada por outro (`POST /panel/v1/price-tables/{versão}/approve`). Os pedidos já orçados mantêm o preço que tinham.

## Componentes

|Serviço|Rede|Papel|
|-|-|-|
|`nginx`|`public` e `internal`|Único com portas publicadas. Faz o TLS, limita requisições por IP (429) e o corpo a 100 MB (413), e distribui entre as réplicas da API|
|`keycloak`|`internal`|Login dos clientes e SSO da equipe, com TOTP obrigatório para a equipe e bloqueio por tentativas. Assina os tokens com EdDSA. Usa o banco `keycloak`, com papel próprio, na instância `postgres`. No logout, avisa a API pelo back-channel|
|`valkey`|`internal`|Estado compartilhado entre as réplicas da API: sessões revogadas e limite de 60 requisições por minuto por usuário. O usuário `default` fica desligado, e o `api` e o `panel` só alcançam as chaves `session:*` e `ratelimit:*`. Se o Valkey cair, a API responde 503|
|`panel`|`internal` e `partners`|Painel interno, em binário próprio. Só aceita tokens do `panel-web` com TOTP (`acr=2`) e perfis da equipe, e grava `panel.access` em toda requisição. A operação crítica "revogar usuário" (só `admin`) exige um login de no máximo 5 minutos e uma justificativa. Grava no banco de auditoria com o papel `panel_audit`, e usa uma service account própria no Keycloak|
|`api`|`internal` e `partners`|Rotas do cliente na porta 8080. Confere o token pelo JWKS do Keycloak (`iss`, `aud`, tipo e perfil `customer`) e a revogação no Valkey, valida o STL, grava no Garage e no Postgres, publica a tarefa e consome os resultados. A porta 8090 é interna: o nginx não a encaminha, e é por ela que o Keycloak faz o back-channel logout|
|`worker`|`processing`|Baixa o arquivo pela URL da mensagem, confere o SHA-256 e calcula o volume e a caixa envolvente. Não alcança banco nem internet e não tem chave do Garage|
|`postgres`|`internal`|Banco principal (`nugget`). Cada módulo e cada binário tem o próprio papel: `files` e `queue` só alcançam `files` e `tasks`, e `orders` (API) e `panel_orders` (painel) alcançam as tabelas de pedidos, com `UPDATE` concedido por coluna, de forma que nenhum dos dois muda um preço pelo SQL|
|`postgres-audit`|`internal`|Eventos de auditoria (banco `audit`, tabela `events`). O papel `audit_app` só tem `INSERT`|
|`rabbitmq`|`internal` e `processing`|Filas quorum `tasks` (até 3 entregas), `tasks.failed` e `results`. Os usuários `api` e `worker` não declaram nada|
|`garage`|`internal` e `processing`|Bucket privado `files`. Só se lê com URL pré-assinada|
|`garage-init`|`internal`|Executa uma vez e prepara o layout, o bucket e a chave da API|
|`partnersim`|`public` e `partners`|Simula o gateway de pagamento (porta 8100) e a transportadora (porta 8200). Exige chave de API, responde de forma assíncrona e avisa a plataforma por webhooks assinados, enviados ao endereço público do nginx. O atraso entre os eventos é o `SIM_DELAY`. A rede `partners` não tem internet e só liga a `api` e o `panel` a ele|

Os estados de uma tarefa são `waiting`, `processing`, `done` e `failed`.

Os binários Go rodam em imagens distroless, como não root, com sistema de arquivos só de leitura, sem capabilities, com `no-new-privileges` e com limites de CPU, memória e processos.

## Testes e verificação

```sh
just test        # testes de unidade e de segurança (TS01 a TS09), sem Docker
just test-e2e    # ponta a ponta contra o compose: precisa de `just up`
just lint        # go vet, gosec, govulncheck e gitleaks
just zap         # varredura do OWASP ZAP pelo openapi.yaml (Etapa 5)
```

O `just test-e2e` cobre:
* o fluxo completo, do upload ao download pelo Garage;
* o 403 para outro cliente;
* URLs adulteradas ou sem assinatura;
* o 413 do nginx;
* a cota de 3 tarefas e a falha segura sem worker (o teste para o worker e depois o religa);
* os papéis do banco;
* o 429;
* o pedido do pagamento até a entrega, com os webhooks do `partnersim`;
* webhooks forjados, alterados, velhos, repetidos e com valor errado;
* o parceiro fora do ar (o teste para o `partnersim` e depois o religa);
* o endereço escondido do técnico.

O `just zap` faz o login do `customer-a` pelo Keycloak e grava os relatórios em HTML, JSON e Markdown em [evidencias/etapa-5/relatorios-zap/](../../evidencias/etapa-5/relatorios-zap/). Cada execução sobrescreve os arquivos anteriores.

## Outros comandos

```sh
just ps             # estado dos contêineres
just logs api       # logs de um serviço
just down           # para tudo
just reset          # para tudo e apaga os volumes: bancos, fila e arquivos
just secrets -force # regera os segredos (depois, `just reset` e `just up`)
```
