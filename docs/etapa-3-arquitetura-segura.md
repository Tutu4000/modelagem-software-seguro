# Etapa 3 — Projeto de uma Arquitetura Segura

Esta etapa transforma os riscos e os controles da [Etapa 2](etapa-2-riscos-nist.md) em requisitos de segurança e decisões de arquitetura. Foi escolhido um risco crítico de cada grupo da [seção 3.1 da Etapa 2][e2-31]:

* **Grupo A, permissão por objeto: [R14].** O vazamento de um projeto não pode ser desfeito e pode atingir todos os clientes.
* **Grupo B, painel interno e privilégios: [R22].** A interface só esconde a opção de aprovar, e o servidor não confere quem faz o pedido.
* **Grupo C, disponibilidade do processamento: [R20].** Sem processamento, nenhum cliente consegue orçar nem comprar.

Os requisitos e as decisões detalham os controles da onda 1 da [ordem de implementação da Etapa 2][e2-46] ([C01] a [C05], [C10] e [C11]). Essa ordem continua valendo. Os limites numéricos (tamanhos, prazos e quantidades) são propostas do grupo, como na Etapa 2.

## 1. Requisitos de segurança

|ID|Risco de origem|Requisito de segurança|Critério de verificação|Controles da Etapa 2|
|-|-|-|-|-|
|RS01|[R14] (Crítico): um usuário baixa STL de outros clientes trocando o identificador na URL|A API deverá obter a identidade do usuário apenas do token de sessão validado e ignorar qualquer identificador de usuário enviado na requisição. Antes de gerar a URL pré-assinada de um STL ou PDF, deverá conferir se o arquivo pertence a um pedido desse usuário ou a um pedido atribuído ao técnico que o solicita. A URL valerá por 5 minutos, e o armazenamento não terá nenhum objeto público|O cliente A pede o arquivo de um pedido do cliente B, recebe 403 e a tentativa fica no log; um `user_id` de outro cliente colocado no corpo ou na URL é ignorado; uma URL usada depois de 5 minutos é recusada; um acesso direto ao armazenamento, sem assinatura, recebe 403|[C01], [C02], [C03]|
|RS02|[R22] (Crítico): um cliente aprova a revisão técnica da própria peça ou acessa funções administrativas|As rotas de aprovação técnica, edição de laudo e administração deverão existir só na API do painel interno, acessível apenas pela VPN ou pela lista de IPs da empresa. Em toda requisição, o servidor deverá conferir o perfil do usuário e, nas aprovações, se o pedido está atribuído a ele, negando tudo o que não estiver explicitamente permitido. Campos de status enviados pelo cliente, como `aprovado` ou `status`, deverão ser ignorados|Um cliente autenticado chama a rota de aprovação com o próprio token, recebe 403 e o evento é registrado; a mesma rota não responde a requisições vindas da internet; um `"status": "aprovado"` enviado pelo cliente não altera o pedido; um teste automatizado percorre todas as rotas da API e falha se alguma não declarar os perfis permitidos|[C01], [C04], [C05]|
|RS03|[R20] (Crítico): muitos modelos de alta densidade enviados ao mesmo tempo sobrecarregam renderização, fatiamento e DFM|Antes de gravar o arquivo e de criar a tarefa, a API deverá conferir o formato real do STL pela estrutura do arquivo, e não pela extensão (no STL binário, o tamanho precisa ser `84 + 50 * n`, sendo `n` o número de triângulos declarado no cabeçalho). Também deverá conferir o tamanho máximo de 100 MB e o limite de 2 milhões de triângulos. Cada conta poderá ter no máximo 3 tarefas simultâneas, e cada tarefa terá tempo e memória máximos. Se a fila lotar ou os workers ficarem indisponíveis, o orçamento ficará "em processamento" e nunca será liberado sem a análise DFM|Um arquivo de 150 MB recebe 413 e nada é gravado; um arquivo com extensão `.stl` e outro conteúdo é recusado; a 4ª tarefa simultânea da mesma conta fica aguardando na fila; uma tarefa que passa do tempo máximo é encerrada; com os workers parados, os orçamentos ficam "em processamento" e nenhum pedido é liberado|[C10], [C11]|

[e2-31]: etapa-2-riscos-nist.md#31-riscos-críticos-12-a-16
[e2-46]: etapa-2-riscos-nist.md#46-ordem-inicial-de-implementação

[R03]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R06]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R14]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R16]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R17]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R19]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R20]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R21]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R22]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R24]: etapa-2-riscos-nist.md#22-registro-de-riscos
[R26]: etapa-2-riscos-nist.md#22-registro-de-riscos

[C01]: etapa-2-riscos-nist.md#c01
[C02]: etapa-2-riscos-nist.md#c02
[C03]: etapa-2-riscos-nist.md#c03
[C04]: etapa-2-riscos-nist.md#c04
[C05]: etapa-2-riscos-nist.md#c05
[C07]: etapa-2-riscos-nist.md#c07
[C08]: etapa-2-riscos-nist.md#c08
[C10]: etapa-2-riscos-nist.md#c10
[C11]: etapa-2-riscos-nist.md#c11
[C13]: etapa-2-riscos-nist.md#c13
[C14]: etapa-2-riscos-nist.md#c14
[C15]: etapa-2-riscos-nist.md#c15
[C16]: etapa-2-riscos-nist.md#c16
[C17]: etapa-2-riscos-nist.md#c17
[C18]: etapa-2-riscos-nist.md#c18
[C19]: etapa-2-riscos-nist.md#c19
[C20]: etapa-2-riscos-nist.md#c20

[CA01]: etapa-1-ameacas-stride.md#ca01--roubo-de-projeto-confidencial
[CA02]: etapa-1-ameacas-stride.md#ca02--arquivo-stl-malicioso
[CA05]: etapa-1-ameacas-stride.md#ca05--autoaprovação-da-revisão-técnica
[CA07]: etapa-1-ameacas-stride.md#ca07--sobrecarga-do-processamento-de-modelos
