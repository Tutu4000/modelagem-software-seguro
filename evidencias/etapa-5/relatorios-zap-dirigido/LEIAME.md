# ZAP dirigido (rodada 4)

Rodada de 14/11/2026, feita depois das três automáticas. O objetivo era alcançar o que elas não alcançaram: o código atrás do limite de requisições e o painel interno.

## Por que outra rodada

Na rodada 3, 37 das 64 respostas de erro que o ZAP registrou eram `429`. O limite de 60 requisições por minuto por usuário barrou a maior parte dos ataques antes do código, e o token de acesso, que vale 5 minutos, venceu no meio da varredura. O painel ficou de fora, porque só escuta no loopback do host.

## Como foi feita

O ZAP 2.16.1 rodou como serviço (`zap.sh -daemon`), na mesma imagem das outras rodadas, e foi controlado pela API dele.

1. **Limites folgados, só no ambiente local.** Um arquivo de override do compose, fora do repositório, subiu o limite por usuário e o limite por IP do nginx. O código e a configuração versionados não mudaram.
2. **Exploração com requisições reais.** Antes de atacar, cada rota recebeu pelo proxy do ZAP uma requisição válida, com IDs e corpos de verdade: upload de um STL, orçamento, envio para revisão, chamado, atribuição, laudo, pagamento, troca de endereço, produção e coleta.
3. **Token sempre válido.** Uma regra do *replacer* do ZAP põe o cabeçalho `Authorization` em toda requisição, e o token é trocado a cada 3 minutos. No painel, cada renovação faz o login com senha e TOTP.
4. **Varredura ativa.** A API do cliente foi varrida como `customer-a`. O painel foi varrido três vezes em paralelo, em três instâncias do ZAP, como `admin`, `technician` e `support`.
5. **Conferência dos logs.** Depois de cada varredura, os logs do nginx, da API e do painel foram conferidos atrás de respostas `5xx`.

## Resultado

| Alvo | Usuário | Requisições | Regras ativas | Duração |
|-|-|-|-|-|
| API do cliente | `customer-a` | 7.490 | 49 | 17 min |
| Painel | `admin` | 11.086 | 49 | 3 min |
| Painel | `technician` | 9.607 | 49 | 3 min |
| Painel | `support` | 9.483 | 49 | 3 min |

A varredura da API rodou a cerca de 9 requisições por segundo, ainda com o limite do nginx. As do painel rodaram sem esse limite.

Respostas que o ZAP recebeu:

| Status | API | Painel |
|-|-|-|
| 400 | 4.571 | 7.275 |
| 403 | 247 | 16.074 |
| 409 | 763 | 1.945 |
| 2xx | 1.589 | 1.483 |
| 404 | 263 | 976 |
| 422 | 7 | 113 |
| 429 | 44 | 2 |
| **500** | **8** | **8** |

No painel, a maioria é `403` porque cada papel só alcança as próprias rotas, e toda operação crítica pede justificativa e login recente.

## Alertas

| Alerta | Risco | Onde | Conclusão |
|-|-|-|-|
| CSP: Failure to Define Directive with No Fallback (10055) | Médio | Respostas da API | A CSP da API não definia `form-action`. **Corrigido:** `form-action 'none'; base-uri 'none'` |
| ZAP is Out of Date (10116) | Baixo | — | É sobre a imagem do ZAP |
| Re-examine Cache-control Directives (10015) | Informativo | API e painel | O mesmo das rodadas anteriores |
| User Agent Fuzzer (10104) | Informativo | API | O ZAP só registra que trocou o `User-Agent`. As respostas não mudaram |

Nenhum alerta de SQL Injection desta vez. Os mesmos ataques foram enviados às mesmas rotas, sem `429` no caminho, o que confirma que o alerta das rodadas 2 e 3 era falso positivo.

## O achado que não virou alerta

As 16 respostas `500` têm a mesma causa:

```text
POST /api/v1/orders                      500  (8 vezes)
POST /panel/v1/orders/:id/address-change 500  (8 vezes)
ERROR: invalid byte sequence for encoding "UTF8": 0x00 (SQLSTATE 22021)
```

Um caractere nulo nos campos do endereço passava pela validação, que conferia só o tamanho, e o PostgreSQL recusava o texto. Reprodução manual, com o nulo em cada campo:

| Campo | Antes | Depois da correção |
|-|-|-|
| `recipient` | 500 | 400 |
| `street` | 500 | — |
| `number` | 500 | — |
| `city` | 500 | 400 |
| Assunto e mensagem de chamado | 400 | 400 |
| Motivo do cancelamento | 400 | 400 |

Depois da correção, conferi `recipient` e `city` pela API, e os quatro campos pelo teste automatizado. A análise está no achado A05 do [documento da Etapa 5](../../../docs/etapa-5-verificacao-vulnerabilidades.md).

## Efeitos no ambiente

A varredura criou centenas de pedidos e chamados de teste, e o `customer-a` atingiu o limite de chamados abertos. Um `just reset` seguido de `just seed` limpa o ambiente.

## Arquivos

- `zap-api-e-painel-admin.*`: a API do cliente e o painel como `admin`;
- `zap-painel-technician.*`: o painel como `technician`;
- `zap-painel-support.*`: o painel como `support`.
