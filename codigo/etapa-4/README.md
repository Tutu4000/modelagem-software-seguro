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
just seed        # envia STLs fictícios dos clientes A e B
```

|Endereço|O que é|
|-|-|
|`https://localhost`|API pública, pelo nginx|
|`https://localhost:8443`|Downloads pelas URLs pré-assinadas do Garage. Só GET e HEAD|
|`https://localhost:8444`|Keycloak, só o realm `nugget`. O console de administração e o realm `master` não são publicados|

O certificado é de uma CA local. Para o `curl`, use `--cacert secrets/tls_ca.pem`. Para o navegador, importe esse arquivo ou aceite o aviso.

No primeiro `just up`, o Keycloak leva cerca de um minuto para criar o banco e importar o realm. A API só sobe depois que ele fica pronto.

### Contas fictícias

O realm é gerado a partir de [deploy/keycloak/realm.json](deploy/keycloak/realm.json) pelo `just secrets`. Todas as contas usam a senha de `secrets/keycloak_demo_password`. As contas da equipe também pedem um código TOTP, gerado a partir de `secrets/keycloak_totp_secret`; para o navegador, cadastre esse segredo num app autenticador.

|Conta|Perfil|TOTP|
|-|-|-|
|`customer-a`, `customer-b`, `customer-c`|`customer`|Não|
|`technician`|`technician`|Sim|
|`support`|`support`|Sim|
|`admin`|`admin`|Sim|

O `customer-c` fica reservado para o teste de cota do e2e. O `just token <conta>` faz o login pelo cliente `e2e`, que só existe no realm local e aceita usuário e senha, e imprime o token. Os front-ends vão usar os clientes `web` e `panel-web`, com authorization code e PKCE.

```sh
TOKEN=$(just token a)
curl --cacert secrets/tls_ca.pem -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: model/stl" --data-binary @peca.stl https://localhost/api/v1/files
curl --cacert secrets/tls_ca.pem -H "Authorization: Bearer $TOKEN" https://localhost/api/v1/files/<id>
```

O contrato das rotas está no [openapi.yaml](openapi.yaml).

## Componentes

|Serviço|Rede|Papel|
|-|-|-|
|`nginx`|`public` e `internal`|Único com portas publicadas. Faz o TLS, limita requisições por IP (429) e o corpo a 100 MB (413), e distribui entre as réplicas da API|
|`keycloak`|`internal`|Login dos clientes e SSO da equipe, com TOTP obrigatório para a equipe e bloqueio por tentativas. Assina os tokens com EdDSA. Usa o banco `keycloak`, com papel próprio, na instância `postgres`. No logout, avisa a API pelo back-channel|
|`valkey`|`internal`|Estado compartilhado entre as réplicas da API: sessões revogadas e limite de 60 requisições por minuto por usuário. O usuário `default` fica desligado, e o `api` só alcança as chaves `session:*` e `ratelimit:*`. Se o Valkey cair, a API responde 503|
|`api`|`internal`|Rotas do cliente na porta 8080. Confere o token pelo JWKS do Keycloak (`iss`, `aud`, tipo e perfil `customer`) e a revogação no Valkey, valida o STL, grava no Garage e no Postgres, publica a tarefa e consome os resultados. A porta 8090 é interna: o nginx não a encaminha, e é por ela que o Keycloak faz o back-channel logout|
|`worker`|`processing`|Baixa o arquivo pela URL da mensagem, confere o SHA-256 e calcula o volume e a caixa envolvente. Não alcança banco nem internet e não tem chave do Garage|
|`postgres`|`internal`|Banco principal (`nugget`). Os papéis `files` e `queue` só alcançam as próprias tabelas, `files` e `tasks`|
|`postgres-audit`|`internal`|Eventos de auditoria (banco `audit`, tabela `events`). O papel `audit_app` só tem `INSERT`|
|`rabbitmq`|`internal` e `processing`|Filas quorum `tasks` (até 3 entregas), `tasks.failed` e `results`. Os usuários `api` e `worker` não declaram nada|
|`garage`|`internal` e `processing`|Bucket privado `files`. Só se lê com URL pré-assinada|
|`garage-init`|`internal`|Executa uma vez e prepara o layout, o bucket e a chave da API|

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
* o 429.

O `just zap` faz o login do `customer-a` pelo Keycloak e grava os relatórios em HTML, JSON e Markdown em [evidencias/etapa-5/relatorios-zap/](../../evidencias/etapa-5/relatorios-zap/). Cada execução sobrescreve os arquivos anteriores.

## Outros comandos

```sh
just ps             # estado dos contêineres
just logs api       # logs de um serviço
just down           # para tudo
just reset          # para tudo e apaga os volumes: bancos, fila e arquivos
just secrets -force # regera os segredos (depois, `just reset` e `just up`)
```
