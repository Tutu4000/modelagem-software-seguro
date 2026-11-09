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
just seed        # dados da demo: pedidos em todos os estados, chamados, troca de endereço, tabela de preço e eventos de segurança
```

|Endereço|O que é|
|-|-|
|`https://localhost`|API pública, pelo nginx|
|`https://localhost:8443`|Downloads pelas URLs pré-assinadas do Garage. Só GET e HEAD|
|`https://localhost:8444`|Keycloak, só o realm `nugget`. O console de administração e o realm `master` não são publicados|
|`https://localhost:9445`|Painel interno, publicado só no loopback do host. As rotas do painel não existem nas outras portas|
|`http://127.0.0.1:8026`|Caixa do Mailpit, com os e-mails enviados aos clientes. Publicada só no loopback do host|
|`https://localhost:9446`|Grafana, publicado só no loopback do host. Usuário `admin`, com a senha em `secrets/grafana_admin_password` (`just grafana`)|

Numa VM, o painel e o Grafana só são alcançados por túnel SSH, que faz o papel da VPN: `ssh -L 9445:127.0.0.1:9445 -L 9446:127.0.0.1:9446 <vm>`.

### Monitoramento

O Grafana tem três dashboards, na pasta `nugget-machine`:

* **Segurança:** lê o banco de auditoria. Mostra negações e as contas com mais negações, uploads recusados, limite de requisições, tokens revogados, acessos ao painel (inclusive fora do horário), operações críticas com a justificativa, webhooks rejeitados por motivo, trocas de endereço e o estado dos alertas.
* **Operação:** lê o Prometheus. Mostra os serviços de pé, as réplicas da API, as requisições e a latência por rota, os 4xx e 5xx, os eventos de auditoria por tipo, as filas, os dois Postgres, o Valkey, o Garage, o Keycloak, o nginx e o runtime Go.
* **Negócio:** lê o banco principal por um papel que não vê dados pessoais. Mostra os pedidos por estado e por dia, a receita, os estornos, o tempo até a entrega, os chamados e os arquivos guardados e apagados.

Os alertas ficam só no Grafana (Alerting e o painel de alertas do dashboard de segurança):

|Alerta|Dispara quando|
|-|-|
|Enumeração de IDs|Uma conta recebe mais de 9 negações em 5 minutos|
|Abuso de upload por uma conta|Uma conta tem mais de 9 uploads recusados ou esperas de cota em 10 minutos|
|Acesso ao painel fora do horário|Alguém acessa o painel entre 20h e 7h, horário de Brasília|
|Operações críticas em série no painel|Uma conta faz mais de 4 operações críticas em 10 minutos|
|Webhooks com assinatura inválida|Mais de 4 notificações forjadas de um parceiro em 10 minutos|
|Serviço fora do ar|Um alvo do Prometheus fica fora por 1 minuto|
|Erros 5xx acima de 5%|A API ou o painel respondem 5xx em mais de 5% das requisições por 2 minutos|
|Mensagens na fila de falhas|`tasks.failed` ou `notifications.failed` têm mensagens|

Para ver o de enumeração disparar, peça uns 15 arquivos que não são seus:

```sh
TOKEN=$(just token e)
for i in $(seq 15); do
  curl -s -o /dev/null --cacert secrets/tls_ca.pem -H "Authorization: Bearer $TOKEN" \
       https://localhost/api/v1/files/$(cat /proc/sys/kernel/random/uuid)
done
```

O certificado é de uma CA local, gerada pelo `just secrets`, com validade de um ano.

* **No `curl`:** use `--cacert secrets/tls_ca.pem`.
* **No navegador:** importe esse arquivo como autoridade confiável (no Firefox, em *Configurações > Privacidade e Segurança > Certificados > Autoridades > Importar*; no Chrome, em *Configurações > Privacidade e segurança > Segurança > Gerenciar certificados*) ou aceite o aviso.

O `just secrets` renova só a cadeia TLS quando ela deixa de servir: sem o Authority Key Identifier, a menos de 30 dias do vencimento ou para outro host. Os outros segredos ficam como estão.

Para outro endereço, como uma VM, rode `just cert <host>`. Ele gera a cadeia com o host no SAN e reinicia o `nginx` e o `partnersim`, sem `reset`. Depois de uma renovação, a CA é nova, então o navegador precisa importá-la de novo.

No primeiro `just up`, o Keycloak leva cerca de um minuto para criar o banco e importar o realm. A API só sobe depois que ele fica pronto.

### O que o `just seed` cria

O seed usa a API e o painel como os clientes e a equipe usariam, e no fim imprime um roteiro de onde olhar. Leva cerca de um minuto. Ele não é idempotente: para começar limpo, `just reset` antes.

* **Pedidos dos clientes A e B em cada estado:** `quoted`, `awaiting_review` (um deles com o alerta de DFM), `awaiting_payment`, `paid`, `in_production`, `delivered` (com rastreio), `rejected` (com laudo) e `cancelled` (com estorno).
* **Chamados:** um respondido pelo suporte, e outro com a troca de endereço proposta pelo suporte, esperando o código que está no Mailpit do `customer-b`.
* **Tabela de preço:** uma proposta do `admin`, com o PLA 10% mais caro, esperando a aprovação do `admin-2`.
* **Eventos de segurança do `customer-c`:** 15 negações, 10 uploads inválidos e um webhook de pagamento forjado. Eles aparecem no dashboard de segurança e disparam os alertas "Enumeração de IDs" e "Abuso de upload". Para não gerá-los, use `go run ./cmd/seed -no-attacks`.

### Contas fictícias

O realm é gerado a partir de [deploy/keycloak/realm.json](deploy/keycloak/realm.json) pelo `just secrets`. Todas as contas usam a senha de `secrets/keycloak_demo_password`. As contas da equipe também pedem um código TOTP, gerado a partir de `secrets/keycloak_totp_secret`; para o navegador, cadastre esse segredo num app autenticador.

|Conta|Perfil|TOTP|
|-|-|-|
|`customer-a` a `customer-g`|`customer`|Não|
|`technician`, `technician-2`|`technician`|Sim|
|`support`|`support`|Sim|
|`admin`, `admin-2`|`admin`|Sim|

O `technician-2` nunca recebe pedidos nos testes, para conferir que só o técnico atribuído trabalha num pedido. O `admin-2` aprova as tabelas de preço que o `admin` propõe, porque uma tabela precisa de dois admins.

O `customer-c` fica reservado para o teste de cota do e2e, e o `customer-d` é revogado pelo teste do painel. O `customer-e` e o `customer-f` servem aos testes de troca de endereço e de retenção, cada um com o próprio limite por usuário. O `customer-g` é revogado pelo teste do painel no navegador e fica desativado até o próximo `just reset`.

O `just token <conta>` faz o login pelo cliente `e2e`, que só existe no realm local e aceita usuário e senha, e imprime o token. Esse token não serve para o painel: sem o fluxo de navegador, o Keycloak não comprova o TOTP. Para o painel, `just panel-token <conta>` faz o login pelo `panel-web`, preenchendo as telas de senha e de TOTP como um navegador. Os apps web usam os clientes `web` e `panel-web`, com authorization code e PKCE. Para entrar pelo navegador:

* **Cliente:** em `https://localhost`, clique em "Entrar" e use `customer-a` com a senha de `secrets/keycloak_demo_password`.
* **Equipe:** em `https://localhost:9445`, use `admin`, `support` ou `technician` com a mesma senha e o código TOTP. Para o código, cadastre a chave num app autenticador (SHA-1, 6 dígitos, 30 segundos). O app pede a chave em base32, e o segredo do realm é texto, então converta antes:

  ```sh
  python3 -c "import base64; print(base64.b32encode(open('secrets/keycloak_totp_secret','rb').read().strip()).decode())"
  ``` O contrato do painel está em [panel-openapi.yaml](panel-openapi.yaml).

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
4. **Admin, no painel:** atribui o pedido a um técnico, escolhido na lista do Keycloak (`GET /panel/v1/technicians` e `POST /panel/v1/orders/{id}/assign`).
5. **Técnico atribuído, no painel:** abre o STL com justificativa (`POST /panel/v1/orders/{id}/file`), que o painel mostra só no visualizador 3D, e registra o laudo e a decisão (`POST /panel/v1/orders/{id}/review`). Aprovado, o pedido vai para `awaiting_payment`; reprovado, para `rejected`.
6. **Cliente:** paga (`POST /api/v1/orders/{id}/payment`). A resposta é a cobrança pendente; o pedido só passa a `paid` quando chega a notificação assinada do gateway em `POST /api/v1/webhooks/payments`.
7. **Técnico atribuído, no painel:** inicia a impressão (`POST /panel/v1/orders/{id}/production`, para `in_production`) e agenda a coleta (`POST /panel/v1/orders/{id}/ship`). O painel manda o endereço para a transportadora sem mostrá-lo ao técnico e recebe o código de rastreio.
8. **Transportadora:** avisa a coleta e a entrega em `POST /api/v1/webhooks/shipping`, e o pedido passa a `shipped` e depois a `delivered`.
9. **Cliente:** acompanha o pedido, o histórico, a revisão, o pagamento e o rastreio (`GET /api/v1/orders/{id}`). Cada etapa também chega ao e-mail cadastrado, sem o endereço.

Depois da compra:

* **Cancelamento:** o cliente cancela até `paid` (`POST /api/v1/orders/{id}/cancel`). Um pedido pago é estornado no gateway; se o gateway estiver fora, a resposta é 202 com o estorno pendente, e uma nova chamada o conclui.
* **Suporte:** o cliente abre chamados, opcionalmente sobre um pedido dele (`POST /api/v1/tickets`), e o suporte responde no painel (`POST /panel/v1/tickets/{id}/messages`). O e-mail só avisa que há resposta.
* **Troca de endereço:** o suporte propõe o endereço novo no painel, a partir do detalhe do pedido ou do chamado (`POST /panel/v1/orders/{id}/address-change`), como operação crítica. O cliente recebe um código de 8 dígitos no e-mail cadastrado e confirma logado (`POST /api/v1/orders/{id}/address-change/confirm`). Sem o código, o endereço não muda, e depois do despacho não há troca.
* **Retenção:** o STL é apagado 90 dias depois da entrega, ou 30 dias depois do upload sem pedido vivo, e o endereço é limpo 90 dias depois da entrega ou do cancelamento. O job roda de hora em hora; `just retention` faz uma passada na hora.

Os webhooks são rotas públicas, sem token: valem pela assinatura `X-Nugget-Signature: t=<unix>,v1=<HMAC-SHA256 de "t.corpo">`, com um segredo por parceiro, e recusam timestamps com mais de 5 minutos. Um evento repetido responde 200 sem efeito, e um evento cujo valor não bate com o pedido responde 400 e fica registrado como `payment.rejected`. Com um parceiro fora do ar, o orçamento e o pagamento respondem 502 `partner_unavailable`, e o pedido não muda.

Os preços mudam por uma tabela nova, proposta por um admin (`POST /panel/v1/price-tables`) e aprovada por outro (`POST /panel/v1/price-tables/{versão}/approve`). Os pedidos já orçados mantêm o preço que tinham. No painel, `/precos/nova` parte da tabela ativa e só muda valores, e o segundo admin vê o que muda antes de aprovar. A revogação de uma conta fica em `/revogar`, só para admin.

## Componentes

|Serviço|Rede|Papel|
|-|-|-|
|`nginx`|`public` e `internal`|Único com portas publicadas. Faz o TLS, limita requisições por IP (429) e o corpo a 100 MB (413), e distribui entre as réplicas da API|
|`keycloak`|`internal`|Login dos clientes e SSO da equipe, com TOTP obrigatório para a equipe e bloqueio por tentativas. Assina os tokens com EdDSA. Usa o banco `keycloak`, com papel próprio, na instância `postgres`. No logout, avisa a API pelo back-channel|
|`valkey`|`internal`|Estado compartilhado entre as réplicas da API: sessões revogadas e limite de 60 requisições por minuto por usuário. O usuário `default` fica desligado, e o `api` e o `panel` só alcançam as chaves `session:*` e `ratelimit:*`. Se o Valkey cair, a API responde 503|
|`panel`|`internal` e `partners`|Painel interno, em binário próprio. Só aceita tokens do `panel-web` com TOTP (`acr=2`) e perfis da equipe, e grava `panel.access` em toda requisição. A operação crítica "revogar usuário" (só `admin`) exige um login de no máximo 5 minutos e uma justificativa. Grava no banco de auditoria com o papel `panel_audit`, e usa uma service account própria no Keycloak|
|`api`|`internal` e `partners`|Rotas do cliente na porta 8080. Confere o token pelo JWKS do Keycloak (`iss`, `aud`, tipo e perfil `customer`) e a revogação no Valkey, valida o STL, grava no Garage e no Postgres, publica a tarefa e consome os resultados. A porta 8090 é interna: o nginx não a encaminha, e é por ela que o Keycloak faz o back-channel logout|
|`worker`|`processing`|Baixa o arquivo pela URL da mensagem, confere o SHA-256 e calcula o volume e a caixa envolvente. Não alcança banco nem internet e não tem chave do Garage. Roda sob o gVisor (`runsc`); numa máquina sem gVisor, suba com `WORKER_RUNTIME=runc just up`|
|`postgres-replica`|`internal`|Réplica de leitura por streaming do `postgres`. Recusa escritas. A lista de pedidos do cliente, o Grafana e o backup leem dela|
|`backup`|`internal`|Uma vez por dia, faz o `pg_dump` da réplica e da auditoria e um `tar` dos arquivos, criptografa tudo com `age` para a chave pública e grava no bucket `backups`, que só a chave dele alcança. Os backups expiram em 30 dias|
|`restore-db` e `restore`|`restore` (e `internal`, para o `restore`)|Só existem durante o `just restore-test`: um Postgres vazio e o restaurador, o único contêiner que recebe a chave privada do `age`|
|`postgres`|`internal`|Banco principal (`nugget`). Cada módulo e cada binário tem o próprio papel: `files` e `queue` só alcançam `files` e `tasks`, `orders` (API) e `panel_orders` (painel) alcançam as tabelas de pedidos, com `UPDATE` concedido por coluna, de forma que nenhum dos dois muda um preço pelo SQL, `support` e `panel_support` alcançam os chamados, e `retention` só lê as datas e apaga o que expirou|
|`postgres-audit`|`internal`|Eventos de auditoria (banco `audit`, tabela `events`). Os papéis `audit_app`, `panel_audit` e `retention_audit` só têm `INSERT`, e o `grafana_reader` só tem `SELECT`|
|`rabbitmq`|`internal`, `processing` e `notify`|Filas quorum `tasks` (até 3 entregas), `tasks.failed`, `results`, `notifications` (até 5 entregas) e `notifications.failed`. Nenhum usuário declara nada: o `panel` só publica avisos, e o `notifier` só os consome|
|`garage`|`internal` e `processing`|Bucket privado `files`. Só se lê com URL pré-assinada|
|`garage-init`|`internal`|Executa uma vez e prepara o layout, o bucket, as chaves da API, do painel e da retenção, e a regra de ciclo de vida do bucket|
|`prometheus`|`internal`|Raspa a API (todas as réplicas, por DNS), o painel, os exporters, o RabbitMQ, o Garage e o Keycloak a cada 15 segundos. Não tem porta publicada. O `/metrics` da API e do painel fica na porta interna 8090, que o nginx não encaminha|
|`grafana`|`internal`|Dashboards e alertas, provisionados por arquivo e só de leitura na interface. Lê o Prometheus e, com os papéis `grafana_reader` e `grafana_main`, os dois bancos|
|`postgres-exporter`, `postgres-audit-exporter`, `valkey-exporter` e `nginx-exporter`|`internal`|Exportam as métricas de cada serviço, com papéis e usuários que só leem estatísticas: `exporter` com `pg_monitor`, o usuário `metrics` do Valkey sem nenhuma chave e o `stub_status` do nginx numa porta só da rede interna|
|`notifier`|`notify`|Consome a fila `notifications` e envia os e-mails por SMTP ao Mailpit. Recebe só o destinatário e o texto: não alcança banco nem Garage, e o log traz só o ID da mensagem|
|`mailpit`|`notify` e `mailui`|Faz o papel do provedor de e-mail. A caixa fica em `127.0.0.1:8026`|
|`retention`|`internal`|Aplica a retenção de hora em hora, com papel de banco, papel de auditoria e chave do Garage próprios. É o único que apaga objetos|
|`partnersim`|`public` e `partners`|Simula o gateway de pagamento (porta 8100) e a transportadora (porta 8200). Exige chave de API, responde de forma assíncrona e avisa a plataforma por webhooks assinados, enviados ao endereço público do nginx. O atraso entre os eventos é o `SIM_DELAY`. A rede `partners` não tem internet e só liga a `api` e o `panel` a ele|

Os estados de uma tarefa são `waiting`, `processing`, `done` e `failed`.

Os binários Go rodam em imagens distroless, como não root, com sistema de arquivos só de leitura, sem capabilities, com `no-new-privileges` e com limites de CPU, memória e processos.

## Testes e verificação

```sh
just test        # testes de unidade e de segurança (TS01 a TS09), sem Docker
just test-e2e    # ponta a ponta contra o compose: precisa de `just up`
just lint        # go vet, gosec, govulncheck e gitleaks
just zap         # varredura do OWASP ZAP da API e do app (Etapa 5; just zap <pasta> muda a saída)
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
* o endereço escondido do técnico;
* os e-mails de cada etapa, sem o endereço;
* o cancelamento, com estorno e com o gateway fora;
* os chamados de suporte;
* a troca de endereço só com o código do e-mail;
* a retenção (o teste recua as datas no banco);
* o isolamento do `notifier`;
* o `/metrics` fora das portas públicas e o Prometheus alcançando todos os serviços;
* o login do Grafana, os dashboards e a consulta à auditoria pelo papel só de leitura;
* os alertas de enumeração e de abuso de upload disparando para a conta certa;
* o worker sob o gVisor;
* a réplica, que recusa escritas e serve a lista de pedidos;
* o backup criptografado, fora do alcance da chave da API, e o restore-test.

O `just zap` faz o login do `customer-a` pelo Keycloak e grava os relatórios em HTML, JSON e Markdown em [evidencias/etapa-5/relatorios-zap/](../../evidencias/etapa-5/relatorios-zap/). Cada execução sobrescreve os arquivos anteriores.

## Front-end

Os dois apps ficam em `web/`, um workspace npm descrito em [FRONTEND.md](FRONTEND.md):

|Pasta|O que é|
|-|-|
|`web/apps/customer`|App do cliente, servido pelo nginx em `https://localhost/`|
|`web/apps/panel`|App do painel, servido em `https://localhost:9445/`|
|`web/packages/ui`|Design system: tokens (bordas retas) e primitivas que se combinam|
|`web/packages/api`|Tipos gerados dos dois OpenAPI, o cliente tipado e os erros em Problem Details|
|`web/packages/auth`|Login com PKCE, sessão e step-up|
|`web/packages/viewer`|Leitor de STL num worker e visualizador 3D|
|`web/packages/domain`|Estados do pedido, linha do tempo, nomes do catálogo, regras de texto e de endereço, comuns aos dois apps|
|`web/e2e`|Testes do Playwright contra o compose|

O nginx constrói os apps na própria imagem (`deploy/nginx/Dockerfile`), então `just up` já os serve. Para desenvolver:

```sh
just web-install        # npm ci --ignore-scripts, a partir do lockfile
just web-dev            # Vite em http://localhost:5173, com a API do compose por proxy
just web-dev panel      # o painel em http://localhost:5174
just web-lint           # tipos gerados em dia, tsc, eslint, vitest e npm audit
just web-e2e            # Playwright contra o compose, na imagem oficial
```

No `just web-dev`, a página `/_design` mostra todas as primitivas. Ela não existe no build de produção.

Para orçar pelo app, entre como cliente e abra "Orçar" (`https://localhost/orcar`):

1. Escolha ou arraste um STL binário. O app lê o arquivo no navegador e mostra a peça em 3D antes do upload terminar.
2. Espere o "Pronto" do processamento.
3. Configure a peça e o endereço.
4. "Calcular orçamento" traz o preço do servidor, com o alerta de DFM quando a peça não cabe na impressora.
5. "Enviar para revisão técnica" encerra o fluxo. Uma recarga no meio do caminho retoma do arquivo enviado.

Em "Pedidos" (`https://localhost/pedidos`), o cliente:

* acompanha cada pedido pela linha do tempo e lê o laudo da revisão;
* paga pelo gateway simulado (a página espera a confirmação), vê o rastreio e cancela até o pagamento, com estorno;
* confirma uma troca de endereço proposta pelo suporte com o código que chegou no e-mail (Mailpit).

Em "Suporte" (`https://localhost/suporte`), o cliente abre chamados, sobre um pedido ou não, e conversa com a equipe. A resposta do suporte aparece sozinha e também chega por e-mail.

O TypeScript é estrito, sem `any`. O ESLint também barra `dangerouslySetInnerHTML`, `innerHTML` e o uso de `localStorage` e `sessionStorage`. Os tipos da API saem do `openapi.yaml` e do `panel-openapi.yaml` (`npm run gen`), e o `just web-lint` falha se estiverem desatualizados.

Todas as portas mandam os cabeçalhos de segurança:

* CSP estrita, sem `unsafe-inline` nem `unsafe-eval` e com Trusted Types;
* HSTS;
* `nosniff`;
* `Referrer-Policy`;
* `Permissions-Policy`;
* COOP e CORP.

O `just zap relatorios-zap-fe0` refaz a varredura da API e do app do cliente.

## Backup e restauração

```sh
just backup        # um backup agora, sem esperar o diário
just restore-test  # restaura o último backup num Postgres temporário e confere
```

O `just restore-test` baixa o backup mais recente e o descriptografa com `secrets/backup_age_identity`. Depois confere o SHA-256 de cada parte contra o manifesto, restaura os dois bancos, compara as contagens de `files`, `orders` e `events` e o número de arquivos, e imprime o tempo total. No fim, o servidor temporário é removido.

## Outros comandos

```sh
just ps             # estado dos contêineres
just logs api       # logs de um serviço
just down           # para tudo
just reset          # para tudo e apaga os volumes: bancos, fila e arquivos
just secrets -force # regera os segredos (depois, `just reset` e `just up`)
```
