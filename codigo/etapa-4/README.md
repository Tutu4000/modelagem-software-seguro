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

O certificado é de uma CA local. Para o `curl`, use `--cacert secrets/tls_ca.pem`. Para o navegador, importe esse arquivo ou aceite o aviso.

```sh
TOKEN=$(just token a)
curl --cacert secrets/tls_ca.pem -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: model/stl" --data-binary @peca.stl https://localhost/api/v1/arquivos
curl --cacert secrets/tls_ca.pem -H "Authorization: Bearer $TOKEN" https://localhost/api/v1/arquivos/<id>
```

O contrato das rotas está no [openapi.yaml](openapi.yaml).

## Componentes

|Serviço|Rede|Papel|
|-|-|-|
|`nginx`|`publica` e `interna`|Único com portas publicadas. Faz o TLS, limita requisições por IP (429) e o corpo a 100 MB (413), e distribui entre as réplicas da API|
|`api`|`interna`|Rotas do cliente. Valida o STL, grava no Garage e no Postgres, publica a tarefa e consome os resultados|
|`worker`|`processamento`|Baixa o arquivo pela URL da mensagem, confere o SHA-256 e calcula o volume e a caixa envolvente. Não alcança banco nem internet e não tem chave do Garage|
|`postgres`|`interna`|Banco principal. Os papéis `arquivos` e `fila` só alcançam as próprias tabelas|
|`postgres-auditoria`|`interna`|Eventos de auditoria. O papel da aplicação só tem `INSERT`|
|`rabbitmq`|`interna` e `processamento`|Filas quorum `tarefas` (até 3 entregas), `tarefas.falhas` e `resultados`. Os usuários `api` e `worker` não declaram nada|
|`garage`|`interna` e `processamento`|Bucket privado `arquivos`. Só se lê com URL pré-assinada|
|`garage-init`|`interna`|Executa uma vez e prepara o layout, o bucket e a chave da API|

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

O `just zap` gera um token do cliente A válido por 2 horas e grava os relatórios em HTML, JSON e Markdown em [evidencias/etapa-5/relatorios-zap/](../../evidencias/etapa-5/relatorios-zap/). Cada execução sobrescreve os arquivos anteriores.

## Outros comandos

```sh
just ps             # estado dos contêineres
just logs api       # logs de um serviço
just down           # para tudo
just reset          # para tudo e apaga os volumes: bancos, fila e arquivos
just secrets -force # regera os segredos (depois, `just reset` e `just up`)
```
