# Backlog de implementação

Plano de ponta a ponta para implementar em Go, com Gin, a arquitetura da [Etapa 3](../../docs/etapa-3-arquitetura-segura.md). O entregável obrigatório da Etapa 4 é só o épico **E1**: duas práticas, com os testes escritos antes do código. Os outros épicos implementam o resto da arquitetura e servem de base para as Etapas 5 a 7. Prazo final: **20/11/2026**.

Os IDs RS, D, FC e os números de seção vêm da [Etapa 3](../../docs/etapa-3-arquitetura-segura.md). Os IDs C, de controle, vêm da [Etapa 2](../../docs/etapa-2-riscos-nist.md#43-catálogo-de-controles).

|Prioridade|Significado|
|-|-|
|P0|Obrigatório para a Etapa 4|
|P1|Base da arquitetura, reaproveitada nas Etapas 5 a 7|
|P2|Completa a arquitetura, se der tempo|
|P3|Evolução, só documentada|

## 1. Stack

|Bloco da Etapa 3|Tecnologia|Observação|
|-|-|-|
|API back-end (monólito modular)|Go + Gin|Dois binários do mesmo código: `api`, com as rotas do cliente, e `panel`, com as rotas internas (seção 1.1)|
|Balanceador de carga|nginx|TLS, limite de requisições e de tamanho, N réplicas da API|
|Identidade|Keycloak, com tokens assinados em EdDSA|A API confere os tokens pelo JWKS e não guarda nenhuma chave privada|
|Banco principal e réplica|PostgreSQL com replicação por streaming|Um papel do banco por módulo|
|Log de auditoria|Instância PostgreSQL separada|A aplicação só tem `INSERT`|
|Estado compartilhado|Valkey|Cotas (C10) e tokens revogados (C08) valem para todas as réplicas|
|Fila|RabbitMQ, com filas quorum|Permissões por usuário: a API publica tarefas e o worker as consome. Tarefas que falham vão para uma fila de falhas|
|Armazenamento de objetos|Garage (compatível com S3)|Bucket privado, URL pré-assinada e permissão por chave. O MinIO comunitário foi descontinuado|
|Workers|Go, na mesma imagem da API|Sem acesso a internet, banco ou Valkey|
|Imagem endurecida|Docker Hardened Images: `dhi.io/golang` (variante dev) no build e `dhi.io/static`, distroless, na execução|Não root, sem shell nem gerenciador de pacotes. O pull exige login com Docker ID|
|Monitoramento|Prometheus e Grafana. O Grafana lê o Prometheus e, por papéis só de leitura, o banco de auditoria e o principal|Três dashboards e oito alertas, base da Etapa 6|
|Segredos|Docker secrets gerados por script|Substituem o cofre no ambiente local|
|CI|GitHub Actions|Base do pipeline da Etapa 7|

### 1.1 Decisões de implementação

|Decisão|Justificativa|Relação com a Etapa 3|
|-|-|-|
|Dois binários do mesmo código: `api`, com as rotas do cliente, e `panel`, com as rotas internas|Contenção: uma falha na API exposta à internet não alcança o código, os segredos nem a rede do painel. O binário `api` não importa os pacotes do painel, então as rotas internas nem existem nele. Cada binário tem réplicas, papel no banco e segredos próprios|Refina o monólito modular da seção 3.2: o código, os módulos e a autorização central continuam únicos, e só a execução se divide. Reforça a D02 (defesa em profundidade) e a decisão de segregação e contenção da aula *Seu sistema é SEGURO ou apenas FUNCIONA?*|
|Cloudflare na frente do nginx (E2.7), com o tráfego de STL passando pelo proxy|Access no lugar da VPN do painel, WAF, mitigação de DDoS e Tunnel, sem porta aberta na origem. Consequência aceita: o Cloudflare termina o TLS e vê os STL em claro, o que exige contrato de tratamento de dados (LGPD)|**Não estava planejado na arquitetura da Etapa 3.** Fica como evolução futura, sem alterar os documentos de arquitetura|
|Erros no formato Problem Details (RFC 9457), com um código estável por tipo de erro|O cliente recebe só o código e um título fixo, e a causa vai para o log. Só o pacote `problem` monta respostas de erro|Segue a D03, segundo a qual mostrar o erro técnico ao cliente exporia detalhes internos (A10:2025)|

## 2. Organização do projeto

```text
codigo/etapa-4/
├── BACKLOG.md
├── README.md                 # como subir o ambiente e rodar os testes
├── justfile                  # secrets, up, down, test, test-e2e, lint, zap (funciona no Windows)
├── go.mod
├── openapi.yaml              # contrato da API, importado pelo ZAP na Etapa 5
├── sqlc.yaml                 # gera o acesso ao banco a partir de migrations/ e dos queries.sql
├── cmd/
│   ├── api/                  # API pública: só as rotas do cliente
│   ├── panel/                # API do painel: só as rotas internas (E6)
│   ├── worker/               # consome a fila e processa os STL
│   ├── garage-init/          # prepara layout, bucket e chave do Garage
│   ├── gensecrets/           # gera os segredos e o certificado TLS local
│   ├── token/                # login de uma conta fictícia pelo Keycloak
│   └── seed/                 # envia STLs fictícios pela API
├── internal/
│   ├── server/               # middlewares compartilhados
│   │   ├── customer/         # roteador da API pública
│   │   └── panel/            # roteador do painel, importado só pelo cmd/panel (E6)
│   ├── config/               # configuração e segredos lidos de arquivo
│   ├── auth/                 # token -> User e revogação
│   ├── authz/                # autorização central
│   ├── files/                # módulo Arquivos: upload e download (filesdb gerado pelo sqlc)
│   ├── stl/                  # validação estrutural do STL
│   ├── fakestl/              # STLs gerados em código para testes e seed
│   ├── storage/              # armazenamento de objetos e URL pré-assinada
│   ├── queue/                # tarefas, resultados e cota por conta (queuedb gerado pelo sqlc)
│   ├── processing/           # lógica do worker
│   ├── review/               # revisão técnica, só no painel (E6)
│   ├── audit/                # eventos de auditoria (auditdb gerado pelo sqlc)
│   ├── devlogin/             # contas fictícias e login pelo cliente e2e, para testes e ferramentas
│   └── problem/              # respostas de erro no formato RFC 9457
├── migrations/
│   ├── main/                 # 01-roles.sh e 02-schema.sql do banco principal
│   └── audit/                # 01-roles.sh e 02-schema.sql do banco de auditoria
├── tests/                    # testes de segurança (TS) e, em e2e/, de ponta a ponta
├── deploy/
│   ├── compose.yaml
│   ├── Dockerfile            # multi-stage, imagem endurecida
│   ├── nginx/
│   ├── rabbitmq/             # rabbitmq.conf e topology.json
│   ├── garage/               # garage.toml
│   ├── backup/               # E3.5
│   └── grafana/              # E7.4
└── secrets/                  # gerado localmente, fora do git
```

O workflow do CI fica em `.github/workflows/`, na raiz do repositório.

As redes do compose reproduzem as fronteiras de confiança:

|Rede|Participantes|O que isola|
|-|-|-|
|`public`|nginx|Único contêiner com portas publicadas: a 443 para os clientes (FC1) e, só em `127.0.0.1`, as do painel e do Grafana, o que simula a VPN (FC2)|
|`internal`|nginx, `api`, `panel`, PostgreSQL, Valkey, RabbitMQ, Garage, Prometheus, Grafana e os exporters|Sem portas publicadas e sem saída para a internet. O Grafana chega ao host pelo nginx, em `127.0.0.1:9446`|
|`processing`|worker, RabbitMQ e Garage|O worker não alcança banco, Valkey nem internet (FC3)|

## 3. Marcos

|Marco|Até|Conteúdo|
|-|-|-|
|M1|09/11|E0 e E1: entregável da Etapa 4|
|M2|13/11|Itens P1 de E2, E3, E4 e E7: o sistema roda no compose e fica pronto para o ZAP da Etapa 5|
|M3|18/11|E9.1 e os itens P2 que couberem|
|Entrega|20/11|Código congelado, README e artigo|

Se o M2 atrasar, a Etapa 5 usa o OWASP Juice Shop, como o enunciado permite.

## 4. Épicos

### E0 — Fundação

|ID|Item|Aceite|Ref.|P|
|-|-|-|-|-|
|E0.1|Módulo Go e pastas. Os placeholders vazios desta pasta são apagados. Os comandos ficam num `justfile`, que também roda no Windows|`go test ./...` roda|—|P0|
|E0.2|`config` lê os segredos de arquivos (`*_FILE`), nunca de variáveis com o valor|A API não sobe se faltar um segredo|3.4|P1|
|E0.3|`just lint` com `go vet`, `gosec`, `govulncheck` e `gitleaks`|Passa sem achados no código do E1|C12|P1|

### E1 — Entregável da Etapa 4 (P0)

Os testes de cada prática entram num commit antes da implementação, junto com funções vazias que retornam erro. Assim `go test` compila e falha, e o histórico mostra a ordem.

**Prática 1 — Autorização por objeto (RS01, D01)**

|Teste|Entrada ou ação|Resultado esperado|
|-|-|-|
|TS01|Cliente A pede a URL do próprio STL|200 com URL pré-assinada|
|TS02|Cliente A pede o STL do cliente B, mesmo enviando `?user_id=B`|403 e evento `authz.denied` no log|
|TS03|Requisição sem token ou com token adulterado|401|
|TS04|URL usada depois de 5 minutos|403|
|TS05|URL com assinatura ou ID de arquivo trocado|403|

**Prática 2 — Validação do upload de STL (RS03, D03)**

|Teste|Entrada ou ação|Resultado esperado|
|-|-|-|
|TS06|Upload de STL binário válido|202 e uma tarefa criada|
|TS07|Arquivo de 150 MB|413 e nada gravado|
|TS08|Arquivo `.stl` com outro conteúdo|422 e nada gravado|
|TS09|STL com mais de 2 milhões de triângulos|422 e nada gravado|

|ID|Item|Aceite|Ref.|
|-|-|-|-|
|E1.1|Testes TS01 a TS05 e funções vazias|`go test` compila e falha|RS01|
|E1.2|`auth`: middleware que valida o JWT e cria o `User`. A identidade vem só do token|TS03|RS01|
|E1.3|`authz`: acesso só do dono ou do técnico atribuído, negação por padrão|TS01, TS02|C01|
|E1.4|`files`: `FindForUser` (não existe busca só por ID) e rota de download|TS01, TS02|D01|
|E1.5|`storage`: URL assinada com HMAC, válida por 5 minutos, conferida em tempo constante. A rota que entrega o arquivo é trocada pelo Garage no E3.3|TS04, TS05|C03|
|E1.6|`audit`: evento de negação, sem token nem dados pessoais|TS02|C17, C18|
|E1.7|Testes TS06 a TS09 e funções vazias|`go test` compila e falha|RS03|
|E1.8|`stl.Parse` e limite de 100 MB com `http.MaxBytesReader`|TS06 a TS09|C10|
|E1.9|O upload grava o arquivo e o hash SHA-256 e entrega só `stl.Validated` à interface `Queue`, que nesta fase fica em memória|TS06|C16, D03|
|E1.10|`docs/etapa-4-codigo-seguro.md`: risco e requisito, testes, trechos do código, resultado esperado e referência OWASP de cada prática|Segue a seção 21 do enunciado|—|

### E2 — Imagem endurecida e ambiente local

|ID|Item|Aceite|Ref.|P|
|-|-|-|-|-|
|E2.1|Dockerfile multi-stage: build com `CGO_ENABLED=0` e `-trimpath`, bases fixadas por digest e um estágio final por binário (`api`, `painel` e `worker`), todos sobre a mesma base endurecida|As imagens rodam como não root, sem shell, e cada uma leva só o próprio binário. O `trivy image` não acha vulnerabilidade crítica|C12|P1|
|E2.2|`compose.yaml` com as redes da seção 2. Todos os contêineres com `no-new-privileges` e limites de memória, CPU e processos. API, painel e worker também com `read_only` e `cap_drop: ALL`|`docker inspect` confirma as opções|3.3, C11|P1|
|E2.3|nginx com TLS (certificado local), `limit_req` por IP e `client_max_body_size 100m`, balanceando as réplicas da API|Com `--scale api=3`, o nginx distribui as requisições. Uma rajada recebe 429|RS03, 3.3|P1|
|E2.4|`cmd/gensecrets` gera os segredos em `secrets/`, que fica no `.gitignore`|O `gitleaks` não acha segredo no repositório|3.4|P1|
|E2.5|`cmd/seed` cria clientes e STLs fictícios, gerados em código|Nenhum dado real no ambiente|3.4|P1|
|E2.6|`openapi.yaml` com as rotas do cliente|O ZAP importa o contrato|—|P1|
|E2.7|Cloudflare na frente do nginx, fora da arquitetura da Etapa 3 (seção 1.1): Access com MFA no lugar da VPN do painel, WAF, mitigação de DDoS e Tunnel. O tráfego de STL passa pelo proxy|A origem só aceita conexões do Tunnel. O nginx limita pelo IP real (`CF-Connecting-IP`), aceito só quando vem do Tunnel. O limite de upload do Cloudflare nos planos Free e Pro (100 MB) coincide com o do RS03|FC1, FC2, D02|P3|

### E3 — Dados, réplica e backup

|ID|Item|Aceite|Ref.|P|
|-|-|-|-|-|
|E3.1|Migrações com UUID nas chaves e um papel do banco por módulo, com GRANT só nas próprias tabelas|Um módulo que lê a tabela de outro recebe permissão negada|C02, FC4|P1|
|E3.2|Repositórios em PostgreSQL no lugar dos em memória. O `FindForUser` filtra pelo dono na própria consulta|`just test-e2e` passa TS01 e TS02 com o banco real|D01|P1|
|E3.3|`storage` passa a usar o Garage: bucket privado e URL pré-assinada do S3|Um acesso direto sem assinatura recebe 403|C03, RS01|P1|
|E3.4|Réplica de leitura por streaming, com um pool separado para listagens e relatórios|Uma escrita na réplica falha. As listagens leem da réplica|3.3|P2|
|E3.5|Backup diário: `pg_dump` da réplica e cópia dos buckets, criptografados com `age`, num bucket separado e com chave própria. O backup expira em 30 dias|A chave da API recebe 403 no bucket de backup|C20, 3.4|P2|
|E3.6|`just restore-test` restaura o último backup num banco temporário e confere as contagens|O teste passa|C20|P2|

### E4 — Fila e processamento isolado

|ID|Item|Aceite|Ref.|P|
|-|-|-|-|-|
|E4.1|RabbitMQ com filas, exchanges, usuários e permissões declarados num arquivo de definições. A API publica tarefas e lê resultados, o worker consome tarefas e publica resultados, e nenhum dos dois altera a topologia|O worker recebe `ACCESS_REFUSED` ao publicar uma tarefa|3.4|P1|
|E4.2|A `queue` publica só `stl.Validated`. A mensagem leva o ID do arquivo, o hash e uma URL de leitura só daquele arquivo|O worker não tem nenhuma chave do Garage|C11, C16|P1|
|E4.3|Cota de 3 tarefas ativas por conta, com contador no Valkey. A 4ª fica em `waiting` e só é publicada quando outra termina|Teste de integração com 4 uploads seguidos|C10, RS03|P1|
|E4.4|O worker confere o hash, processa com tempo máximo e `GOMEMLIMIT` dentro do limite do contêiner e simula o fatiamento (volume e caixa envolvente)|Uma tarefa que estoura o tempo é encerrada. Um hash diferente é recusado|C11|P1|
|E4.5|Falha segura: a fila quorum entrega cada tarefa no máximo 3 vezes (`x-delivery-limit`) e depois a manda para a fila de falhas (dead-letter exchange). Sem worker, o orçamento fica "em processamento"|Com o worker parado, nenhum orçamento é liberado|D03|P1|
|E4.6|A API consome os resultados e grava no banco, de forma idempotente pelo ID da tarefa|Um resultado repetido não duplica o registro|3.4|P1|
|E4.7|Autoscaler pelo tamanho da fila, com teto de custo|Exigiria Kubernetes e KEDA. Localmente, `--scale worker=N`|3.3|P3|
|E4.8|Sandbox sem rede para cada tarefa (gVisor ou Firecracker)|—|C11|P3|

### E5 — Sessão e estado compartilhado

|ID|Item|Aceite|Ref.|P|
|-|-|-|-|-|
|E5.1|Tokens de acesso de 15 minutos. O logout ou um incidente põem o token numa lista de revogados no Valkey, consultada por todas as réplicas|O token revogado recebe 401 em qualquer réplica|C08, 3.3|P2|
|E5.2|Limite de requisições por usuário no Valkey, além do limite por IP do nginx|Um script de enumeração recebe 429|C19|P2|
|E5.3|Keycloak: login dos clientes com limite de tentativas e SSO da equipe com MFA obrigatório|—|C05, C07|P3|

### E6 — Painel interno

|ID|Item|Aceite|Ref.|P|
|-|-|-|-|-|
|E6.1|Binário `panel` só na rede interna, com lista de IPs. O binário `api` não contém nenhuma rota do painel|Uma rota do painel chamada pela porta pública recebe 404|RS02, D02|P2|
|E6.2|`RequireRole` em todo grupo de rotas, e um teste que percorre as rotas dos dois roteadores|O teste falha quando alguém adiciona uma rota sem perfil declarado|RS02|P2|
|E6.3|Aprovação técnica só pelo técnico atribuído, ignorando o `status` enviado pelo cliente, com uma aprovação por pedido|O cliente com o próprio token recebe 403. Uma aprovação repetida não duplica|RS02, 3.4|P2|
|E6.4|Operações críticas com MFA de novo (TOTP), justificativa obrigatória e alerta|Sem o código TOTP, a operação recebe 403|D02|P3|

### E7 — Logs, auditoria e monitoramento

|ID|Item|Aceite|Ref.|P|
|-|-|-|-|-|
|E7.1|Catálogo de eventos (`authz.denied`, `upload.rejected`, `quota.waiting`, `token.revoked`, `auth.revoked_token_used`, `rate.limited`, `panel.access`, `panel.critical`, `user.revoked`, `order.created`, `order.submitted`, `order.assigned`, `review.approved`, `review.rejected`, `file.staff_download`, `price_table.proposed`, `price_table.approved`, `payment.requested`, `payment.succeeded`, `payment.rejected`, `order.in_production`, `shipment.requested`, `shipment.picked_up`, `shipment.delivered`, `shipment.rejected`, `shipment.exception`, `notify.failed`, `order.cancelled`, `payment.refund_requested`, `payment.refunded`, `ticket.created`, `ticket.replied`, `ticket.closed`, `address.change_requested`, `address.change_failed`, `address.changed`, `file.purged`, `address.purged`) com os campos da seção 3.2|Todo evento tem autor, alvo, operação, horário UTC, origem e resultado|C18|P1|
|E7.2|Banco de auditoria separado, em que o papel da aplicação só tem `INSERT`|`UPDATE` e `DELETE` feitos pela aplicação recebem permissão negada|C18|P1|
|E7.3|Logs da aplicação em JSON (`slog`), sem senha, token, cartão ou conteúdo de arquivo|Um teste procura o token da requisição no log e falha se encontrar|C17|P1|
|E7.4|Grafana com papel só de leitura no banco de auditoria e um painel de negações, uploads recusados e acessos ao painel|O painel mostra os eventos do seed|C19|P2|
|E7.5|Três alertas: enumeração de IDs, abuso de upload por uma conta e acesso ao painel fora do padrão. São as regras da Etapa 6|Um script de enumeração dispara o alerta|C19|P2|
|E7.6|Auditar as consultas ao próprio log e centralizar os logs operacionais (Loki)|—|3.2|P3|

### E8 — Integrações e regras de negócio (P3)

|ID|Item|Ref.|
|-|-|-|
|E8.1|Pedidos e orçamento: preço recalculado no servidor e tabela de preços versionada|C13, 3.4|
|E8.2|Pagamentos com gateway simulado: assinatura da notificação e idempotência pelo ID da transação. Com o gateway fora, o pedido fica "aguardando pagamento"|C14, 3.4|
|E8.3|Logística com retorno assinado por HMAC|C15|
|E8.4|Notificações só com o destinatário e o texto|3.4|
|E8.5|Retenção: STL apagado 90 dias depois da entrega, com regra de expiração nos buckets|3.4|
|E8.6|Aplicação web e visualizador 3D com CSP|C09|

### E9 — Pipeline (Etapa 7)

|ID|Item|Aceite|Ref.|P|
|-|-|-|-|-|
|E9.1|`ci.yml` com `go vet`, testes, `gosec`, `govulncheck`, `gitleaks`, build da imagem e `trivy`. O token do Docker Hub fica num segredo do GitHub|Qualquer falha bloqueia o merge. São as condições de parada da Etapa 7|C12|P1|
|E9.2|SBOM da imagem em CycloneDX|Gerado a cada build|C12|P2|
|E9.3|ZAP baseline contra o ambiente do compose|O relatório fica anexado ao build|—|P2|

## 5. Simplificações do ambiente local

A aula *Por que uma arquitetura segura ainda pode resultar em software vulnerável?* mostra que a implementação pode enfraquecer uma decisão sem querer. Por isso as simplificações ficam explícitas:

|Na arquitetura da Etapa 3|No ambiente local|
|-|-|
|Workers sem rede|Rede `processing`, sem internet, banco nem Valkey. Para não ter rede nenhuma, seria preciso o sandbox do E4.8|
|Autoscaler com teto de custo|`--scale worker=N` manual (E4.7)|
|IdP dos clientes e SSO com MFA|Keycloak no compose (E5.3). Um cliente `e2e` com login por usuário e senha existe só no realm local, para os testes e as ferramentas|
|VPN do painel|Porta do nginx publicada só em `127.0.0.1:9445`, e o nginx só aceita o gateway da rede `public`, por onde chegam o loopback e um túnel SSH. A evolução prevista é o Cloudflare Access (E2.7)|
|MFA no token do painel|Só o login pelo navegador (`panel-web`) comprova o TOTP, com `acr=2` e `auth_time`. Tokens obtidos por usuário e senha trazem `acr=1` mesmo para a equipe com TOTP, e o painel os recusa|
|Step-up nas operações críticas (E6.4)|Login de no máximo 5 minutos, pedido pelo cabeçalho `WWW-Authenticate` da RFC 9470, no lugar de um segundo código TOTP dentro da mesma sessão|
|Service account do painel no Keycloak|Para revogar um usuário, o painel desativa a conta e encerra as sessões pela Admin API, com o papel `manage-users`. É o papel mais estreito que o Keycloak oferece e também permite editar usuários: **risco aceito**. O segredo só existe no contêiner do painel, a Admin API não é publicada, e cada uso fica no evento `user.revoked`. A mesma conta confere se quem recebe um pedido é técnico|
|DFM no worker (E4.4)|O alerta `exceeds_build_volume` é calculado no módulo de pedidos, a partir da caixa envolvente que o worker já devolve, contra um volume de impressão de 250 mm por eixo (`BUILD_VOLUME_MM`). A mensagem da fila não muda|
|Aprovar sem laudo, só pelo líder de produção (D02)|Não existe. O MVP não tem o perfil de líder de produção, então toda aprovação exige laudo e técnico atribuído|
|Papéis do painel|O painel usa papéis próprios no banco (`panel_orders`, `panel_audit`), uma chave do Garage só de leitura e um usuário próprio no Valkey. O `UPDATE` em `orders` é concedido por coluna: nem a API nem o painel conseguem mudar um preço pelo SQL|
|Cofre de segredos|Docker secrets|
|Banco criptografado em repouso|Não implementado localmente. Em produção, disco criptografado do provedor|
|Backup em outra conta|Bucket separado, com uma chave que a API não tem|
|Serviços externos (FC5)|Gateway e transportadora simulados pelo contêiner `partnersim`, que se comporta como terceiro: chave de API nas chamadas, resposta assíncrona e webhooks assinados (E8.2, E8.3). A API e o painel chegam a ele por uma rede própria, `partners`, sem internet|
|Webhooks dos parceiros|Rotas públicas pelo nginx, sem JWT, como numa plataforma real. A autenticação é o cabeçalho `X-Nugget-Signature` (HMAC-SHA256 de `t.corpo`, com um segredo por parceiro e tolerância de 5 minutos). O registro de rotas declara o esquema `hmac:payments` ou `hmac:shipping` no lugar dos perfis, e o teste de cobertura aceita só esses casos. O ID do evento é a chave de idempotência|
|Endereço de entrega|Só o mínimo para entregar: destinatário, logradouro, número, cidade, UF e CEP. O cliente, o suporte e o admin o veem; o técnico não. Vai para a transportadora só na coleta|
|Provedor de e-mail (E8.4)|Mailpit no compose, com a caixa publicada só em `127.0.0.1:8026`. O `notifier` recebe pela fila só o destinatário, o assunto e o texto, numa rede (`notify`) onde estão só ele, o RabbitMQ e o Mailpit. O SMTP interno não tem TLS nem autenticação. A entrega é pelo menos uma vez: um e-mail pode chegar repetido, com o mesmo `Message-ID`|
|Canal cadastrado (C21)|O e-mail verificado no Keycloak, copiado para o pedido no orçamento. A troca de endereço proposta pelo suporte só vale com o código de 8 dígitos enviado a esse e-mail e digitado pelo dono, logado. O código fica guardado só como hash, vale 30 minutos e trava a troca depois de 5 erros. Não há confirmação por telefone|
|Frete depois da troca de endereço|Não é cotado de novo: o pedido mantém o frete do orçamento|
|Cancelamento (RF10)|O cliente cancela até `paid`. Um pedido pago muda para `cancelled` antes do estorno, na mesma transação da cobrança, para não perder a corrida com o técnico; se o gateway estiver fora, o estorno fica pendente e uma nova chamada o conclui com a mesma chave. Da produção em diante, o cancelamento é com o suporte, fora do sistema|
|Retenção (E8.5)|Um job próprio (`retention`), com papel de banco, papel de auditoria e chave do Garage próprios. Apaga o STL 90 dias depois da entrega e o STL sem pedido vivo depois de 30 dias, e limpa o endereço 90 dias depois da entrega ou do cancelamento. O Garage não tem permissão só de apagar, então a chave do job também escreve: **risco aceito**, a chave existe só nesse contêiner. A regra de ciclo de vida do bucket (expiração em 400 dias e aborto de upload incompleto em 1 dia) é só a rede de segurança, aplicada pelo `garage-init` com uma chave `owner` temporária, apagada em seguida|
|Login no Grafana|Usuário `admin` local, com a senha gerada pelo `gensecrets`, a proteção contra força bruta do Grafana e o mesmo isolamento de rede do painel (`127.0.0.1:9446`, só o gateway da rede `public`). Não há SSO com o Keycloak|
|Notificação dos alertas (E7.5)|Os alertas ficam só no Grafana: o dashboard de segurança mostra o estado, e a API de alertas é o que o e2e confere. Não há contact point, e-mail nem plantão|
|Métricas por contêiner|Sem cAdvisor, que pediria root, o socket do Docker e o `/sys` do host. Cada serviço expõe as próprias métricas, e o worker, o `notifier` e o `retention` não são raspados: o estado deles aparece nas filas do RabbitMQ e nos eventos de auditoria|
|Detalhe por conta nas métricas|As métricas do Prometheus usam só a rota registrada e o nome do evento, para a cardinalidade não crescer com os usuários. O detalhe por conta, de que os alertas de enumeração e de upload precisam, vem do banco de auditoria, pelo papel `grafana_reader`|
|Ator das mudanças feitas pelos parceiros|O histórico de estados guarda `uuid.Nil`, e o evento de auditoria leva `partner:payments` ou `partner:shipping` como autor|
|Docker Hardened Images|`golang` e `gcr.io/distroless/static-debian13:nonroot`, fixadas por digest. Continuam sem shell e sem root, e não exigem login com Docker ID|
|Cota no Valkey (E4.3)|Cota no Postgres, com `pg_advisory_xact_lock` por conta. Também vale para todas as réplicas. O Valkey guarda só as revogações e o limite por usuário (E5)|
|Tokens de 15 minutos (E5.1)|Tokens de 5 minutos. A revogação vale por 10 minutos, o que cobre todos os tokens emitidos antes dela|
|Limite por usuário (E5.2)|Janela deslizante aproximada por dois contadores de janela fixa, num script Lua atômico. É mais barata que guardar cada requisição e erra pouco perto da virada da janela|
|Valkey indisponível|A API falha fechada: responde 503, porque não consegue conferir a revogação (D03). O preço é a API parar junto com o Valkey|
|Migrações versionadas|SQL em `migrations/`, executado pelo Postgres só na criação do volume. Para mudar o esquema, `just reset`|
|Makefile e scripts em shell|`justfile` e comandos em Go (`cmd/gensecrets`, `cmd/token`, `cmd/seed`), que rodam no Windows|
|TLS entre os serviços|Só na borda, no nginx. A rede interna fica sem TLS|
|Admin API do Garage só para o init|Escuta também na rede de processamento. Exige o token, que o worker não tem|
|Outbox transacional|A tarefa é publicada antes do commit, e o resultado só recebe ack depois dele. Se o commit falhar, nenhuma tarefa fica parada, mas o worker pode processar uma tarefa que não existe, ou a mesma duas vezes. A idempotência do E4.6 descarta o resultado extra|
|Cabeçalhos de segurança no nginx|Ficam de fora de propósito, para a verificação da Etapa 5 encontrá-los. A correção é registrada lá|

## 6. Ligação com as próximas etapas

|Etapa|O que o backlog entrega|
|-|-|
|5 — Verificação|O sistema rodando no compose (M2) e o `openapi.yaml` (E2.6), para o ZAP testar o nosso próprio sistema|
|6 — Detecção|O catálogo de eventos (E7.1) como fonte de dados, o dashboard de segurança (E7.4) e os alertas do E7.5 (`deploy/grafana/provisioning/alerting/rules.yaml`) como evidência|
|7 — DevSecOps|O pipeline do E9.1, com as condições que bloqueiam o merge|

## 7. Estado

|Marco|Estado|O que entrou|
|-|-|-|
|M1|Concluído|E0.1 e E1|
|M2|Concluído, em versão simples|E0.2, E0.3, E2.1 a E2.6, E3.1 a E3.3, E4.1 a E4.6 e E7.1 a E7.3, com as simplificações da seção 5. O `just test-e2e` cobre os aceites|
|M3|Pendente|E9.1 e os itens P2|
|E6.1, E6.2 e E6.4|Concluídos|Binário `panel` próprio, em `127.0.0.1:9445`, com TOTP exigido no token e evento `panel.access` em toda requisição. Toda rota declara os perfis num registro, e um teste falha se alguma rota não declarar. O binário `api` não depende do painel. A operação crítica "revogar usuário" exige step-up e justificativa|
|E8.1 e E6.3|Concluídos|Pedido com orçamento calculado só no servidor, que recusa campos desconhecidos no corpo (CA03). Tabela de preços versionada, que só vale depois da aprovação de outro admin, e pedidos que mantêm o preço do orçamento. Revisão técnica só pelo técnico atribuído, com laudo, uma por pedido (CA05). Download do STL pela equipe só pelo técnico atribuído, como operação crítica (CA08). Histórico de estados e alerta de DFM|
|E7.4 e E7.5|Concluídos|Prometheus raspando a API (todas as réplicas, por DNS), o painel, os dois Postgres, o Valkey, o RabbitMQ (por fila), o Garage, o Keycloak e o nginx, sem nenhuma porta publicada. Grafana com os dashboards de segurança, operação e negócio, provisionados e só de leitura. O papel `grafana_reader` só lê a auditoria, e o `grafana_main` não vê dados pessoais. Oito alertas no Grafana: enumeração de IDs, abuso de upload, acesso ao painel fora do horário, operações críticas em série, webhooks forjados, serviço fora, erros 5xx e fila de falhas. O e2e dispara os de enumeração e de upload|
|E8.4 e E8.5|Concluídos|Avisos ao e-mail cadastrado em cada etapa do pedido, sem o endereço, pelo `notifier`, que não alcança banco nem Garage. Cancelamento pelo cliente até `paid`, com estorno no gateway (RF10, CA06, T11). Chamados de suporte só sobre os próprios pedidos, respondidos pelo suporte no painel (RF11). Troca de endereço proposta pelo suporte como operação crítica e confirmada pelo cliente com o código enviado ao e-mail (C21, CA04). Problema na entrega avisado ao cliente (RF15). Retenção do STL e do endereço por job próprio, com o ciclo de vida do bucket como rede de segurança|
|E8.2 e E8.3|Concluídos|Frete cotado na transportadora e somado depois do desconto. Pagamento pelo total calculado no servidor, com uma cobrança viva por pedido. O pedido só fica pago com a notificação assinada do gateway, e o valor precisa bater com o total (`payment.rejected` com `amount_mismatch` quando não bate). Evento repetido não tem efeito. Produção e coleta pelo técnico atribuído, coleta e entrega pelos webhooks da transportadora. Com o parceiro fora, 502 `partner_unavailable` e o pedido não muda (D03)|
|E5.1 e E5.2|Concluídos|Valkey com ACL por usuário. O back-channel logout do Keycloak revoga a sessão em todas as réplicas, e a revogação por usuário já existe para o painel (F3). Limite de 60 requisições por minuto por usuário, com 429 e `Retry-After`. Sem o Valkey, 503|
|E5.3|Concluído|Keycloak com realm versionado, quatro perfis, TOTP obrigatório para a equipe, bloqueio por tentativas, tokens EdDSA de 5 minutos e clientes `web` e `panel-web` com PKCE. A API confere `iss`, `aud`, tipo e perfil, e um token válido da equipe recebe 403 na API do cliente|
