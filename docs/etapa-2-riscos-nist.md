# Etapa 2 - Análise e Priorização de Riscos de Segurança

## Parte 1 - Mapeamento das fontes da documentação

### 1.1 Processos de negócio e perfis

* **Pedido:** o cliente envia o STL, configura a peça, recebe o orçamento depois do fatiamento e da análise DFM, paga e acompanha a produção e a entrega (RF02 a RF08, RF12, RF19).
* **Revisão técnica:** o técnico emite o laudo DFM e aprova ou reprova o projeto pelo painel interno (RF19, RF20).
* **Pós-venda:** histórico, cancelamento, suporte e avaliação (RF09 a RF11, RF13).
* **Perfis:** o cliente é um usuário externo e não confiável. Atendentes, técnicos e administradores são usuários internos com acesso privilegiado.

### 1.2 Fluxos de dados

|ID|Fluxo|Caminho|Dados|
|-|-|-|-|
|F1|Autenticação|Cliente -> Aplicação Web -> API -> Serviço de Autenticação|Credenciais e sessão|
|F2|Upload e processamento|Cliente -> API -> Armazenamento -> Servidores de processamento -> Armazenamento|STL, PDF, prévias e laudos|
|F3|Pedidos|API <-> Banco de dados|Cadastro, pedidos, preços, status e histórico|
|F4|Checkout|API <-> Gateway de pagamento|Valores e transações|
|F5|Logística|API <-> API da transportadora, incluindo o retorno de rastreio|Endereço, frete e rastreio|
|F6|Operação interna|Técnico, suporte ou administrador -> Painel interno -> API|Laudos, aprovações, status, endereços e preços|
|F7|Notificações|API -> serviço de e-mail e SMS|Avisos de problemas (RF14 a RF17)|

### 1.3 Componentes e ativos críticos

|Componente ou ativo|Motivo|Fluxos|
|-|-|-|
|Armazenamento de arquivos (STL, PDF e laudos)|Guarda a propriedade intelectual dos clientes. Um vazamento não pode ser desfeito|[F2]|
|Servidores de processamento (renderização, fatiador e DFM)|Processam arquivos enviados por qualquer cliente, e todo orçamento depende deles|[F2]|
|Painel interno|Dá acesso aos projetos de todos os clientes e às aprovações técnicas|[F6]|
|API de back-end|Concentra as regras de negócio e as verificações de permissão|[F1] a [F7]|
|Serviço de autenticação e sessões|Todos os perfis entram por ele|[F1], [F6]|
|Banco de dados|Guarda CPF/CNPJ, endereços, pedidos, preços e status|[F3]|
|Integração com o gateway de pagamento|A confirmação do pagamento libera o pedido para produção|[F4]|
|Logs de auditoria|São a única evidência em disputas e na investigação de incidentes|Todos|

### 1.4 Ameaças complementares

Ao percorrer os fluxos, encontramos quatro ameaças que não constam da Etapa 1. Elas foram registradas na Parte 2 como TC01 a TC04:

* **TC01:** a Etapa 1 não trata da confirmação do pagamento ([F4]), que é o que libera o pedido para produção ([seção 4 da Etapa 1][e1-4], passo 5).
* **TC02:** mensagens de suporte e avaliações foram classificadas como ativo não importante ([seção 3.2 da Etapa 1][e1-32]), mas aparecem no painel interno ([F6]) e podem ser usadas para atacar a equipe.
* **TC03:** os logs de auditoria são considerados ativo importante ([seção 3.2 da Etapa 1][e1-32]), mas nenhuma ameaça trata da integridade deles. As ameaças de repúdio [T10], [T11] e [T12] dependem disso.
* **TC04:** o PDF do RF22 só foi analisado como alvo de vazamento ([T17]). Ele também pode ser um arquivo malicioso aberto por um técnico.

## Parte 2 - Registro e categorização dos riscos

### 2.1 Critérios

**Probabilidade**

|Valor|Classificação|Critério|
|-|-|-|
|1|Baixa|Evento raro, que exige condições de rede ou de acesso incomuns, configurações atípicas ou alta capacidade técnica|
|2|Média-baixa|Evento possível, mas que depende de falhas de configuração ou de cenários específicos da arquitetura|
|3|Média-alta|Evento provável no uso cotidiano do sistema ou em ataques frequentes|
|4|Alta|Evento muito provável, fácil de executar ou previsível pela falta de controles na arquitetura atual|

**Impacto**

|Valor|Classificação|Critério|
|-|-|-|
|1|Baixo|Transtorno pontual, corrigido rapidamente e sem prejuízo operacional ou financeiro|
|2|Moderado|Interrupção temporária ou inconsistência local, com recuperação clara dos dados|
|3|Alto|Prejuízo financeiro ou administrativo relevante, ou vazamento de dados pessoais ou sensíveis|
|4|Muito alto|Indisponibilidade de serviços essenciais, perda irreversível de dados vitais ou dano institucional grave que atinge vários usuários ao mesmo tempo|

**Classificação**

`Pontuação = Probabilidade x Impacto`

|Pontuação|Nível|
|-|-|
|1 a 3|Baixo|
|4 a 7|Moderado|
|8 a 11|Alto|
|12 a 16|Crítico|

As notas partem de duas premissas:

* O sistema ainda está em projeto e a Etapa 1 não diz quais controles existem. As condições descritas nos casos de abuso foram tratadas como plausíveis, mas não como certas, por isso nenhuma ameaça recebeu probabilidade 4.
* Vazamento de dados pessoais recebeu impacto 3, como a escala indica. O impacto 4 ficou para eventos irreversíveis ou que atingem todos os clientes de uma vez: vazamento de projetos, alteração de pedidos em massa, parada do processamento e controle do painel interno.

### 2.2 Registro de riscos

Cada linha é um risco, identificado pelo ID da ameaça de origem.

|Arquivo / Origem|Ameaça (STRIDE)|Vulnerabilidade / Falta de Controle|Probabilidade (1-4)|Impacto (1-4)|Pontuação (P x I)|Criticidade|
|-|-|-|-:|-:|-:|-|
|[T01]|Spoofing: alguém entra na conta do cliente com credenciais vazadas ou sessão sequestrada (RF01)|A arquitetura não prevê MFA, limite de tentativas de login, alerta de acesso suspeito nem regras de sessão (expiração, troca de token, cookies `HttpOnly` e `Secure`)|3|3|9|Alto|
|[T02], [CA04]|Spoofing: um golpista se passa pelo cliente no suporte e troca o endereço de entrega (RF11)|O suporte confirma a identidade só com nome, CPF e número do pedido, que são dados fáceis de conseguir, e não confirma a troca pelo canal cadastrado|3|3|9|Alto|
|[T03], [CA08]|Spoofing: um atacante usa as credenciais de um técnico para entrar no painel interno (RF20)|O painel não exige MFA nem restringe a rede de acesso, e o processo de controle de acesso dos administradores e da equipe interna (criação, revisão e remoção de contas) não está documentado|3|4|12|Crítico|
|[T04]|Spoofing: alguém se passa pela transportadora e envia valores de frete ou status de entrega falsos (RF07)|O retorno de rastreio da transportadora para a API não tem autenticação de origem (assinatura HMAC, mTLS ou lista de IPs)|2|2|4|Moderado|
|[T05], [CA02]|Tampering: o STL é alterado no envio ou no armazenamento antes do fatiamento (RF02)|Não há conferência de integridade (hash) entre upload, armazenamento e produção, e o uso de TLS não está especificado|2|3|6|Moderado|
|[T06], [CA06]|Tampering: um cliente cancela pedidos de outros clientes ou altera o status deles (RF10, RF12, RF20)|Os números de pedido são sequenciais e o servidor não confere se o pedido pertence a quem fez a requisição (IDOR)|3|4|12|Crítico|
|[T07], [CA03]|Tampering: o cliente altera o preço na requisição de checkout (RF06, RF23)|O servidor pode aceitar o preço, o material ou a quantidade enviados pelo navegador sem refazer o cálculo|3|3|9|Alto|
|[T08], [CA03]|Tampering: o cliente informa uma quantidade falsa para ganhar o desconto progressivo (RF24)|O desconto é calculado com parâmetros enviados pelo cliente, sem conferir a quantidade efetivamente paga|3|2|6|Moderado|
|[T09], [CA08]|Tampering: o laudo DFM é alterado antes de chegar ao cliente (RF19, RF20)|O laudo fica no armazenamento sem conferência de integridade nem histórico de versões, e as edições não registram o autor|2|3|6|Moderado|
|[T10]|Repudiation: o cliente contesta um pagamento e não há registro que comprove a transação (RF08)|O ID da transação no gateway não é vinculado ao pedido e não há conciliação|2|2|4|Moderado|
|[T11], [CA06]|Repudiation: o cliente nega ter pedido o cancelamento (RF10)|O cancelamento não registra quem o fez (usuário, data e IP) e não gera confirmação no canal cadastrado|3|2|6|Moderado|
|[T12], [CA08]|Repudiation: o técnico nega ter aprovado um projeto com falha (RF20)|As aprovações não ficam registradas de forma nominal e imutável, e a produção pode usar contas compartilhadas|2|3|6|Moderado|
|[T13]|Repudiation: o usuário nega ter escrito uma avaliação ofensiva (RF13)|A avaliação não fica registrada junto à conta e à sessão que a enviaram|2|1|2|Baixo|
|[T14], [CA01]|Information Disclosure: um usuário baixa STL de outros clientes trocando o identificador na URL (RF02, RF03)|Os arquivos são acessados por URL previsível ou pública, sem URL pré-assinada e sem conferência do dono|3|4|12|Crítico|
|[T15]|Information Disclosure: dados de pagamento aparecem em logs ou na API de checkout (RF08)|Não há regra de mascaramento nos logs, e as permissões da API de checkout não foram definidas|2|3|6|Moderado|
|[T16]|Information Disclosure: um usuário consulta histórico, cadastro (CPF e endereço) ou laudos de outros clientes (RF09, RF10, RF20)|O mesmo IDOR do [T06], agora em consultas, com IDs sequenciais|3|3|9|Alto|
|[T17], [CA01]|Information Disclosure: desenhos técnicos em PDF, com segredos industriais, são acessados sem autorização (RF22)|Os PDFs ficam no mesmo armazenamento do [T14], sem separação por cliente, e os técnicos têm acesso a todos os arquivos|3|4|12|Crítico|
|[T18]|Denial of Service: tentativas de força bruta no login bloqueiam usuários legítimos (RF01)|Não há limite de tentativas por IP ou por conta, e um bloqueio de conta mal desenhado pode ser usado para impedir o acesso de terceiros|3|2|6|Moderado|
|[T19], [CA07]|Denial of Service: envio em massa de arquivos, zip bombs e modelos gigantes esgotam o armazenamento e o processamento (RF02)|Não há limite de tamanho, de envios por conta nem de espaço, e o formato não é validado antes da gravação|3|3|9|Alto|
|[T20], [CA07]|Denial of Service: muitos modelos de alta densidade enviados ao mesmo tempo sobrecarregam renderização, fatiamento e DFM (RF03, RF05, RF06, RF19)|O processamento não tem fila, cota por usuário, limite de polígonos nem tempo máximo por tarefa|3|4|12|Crítico|
|[T21], [CA02]|Elevation of Privilege: um STL malicioso explora uma falha do fatiador ou do visualizador e executa código no servidor (RF02, RF03)|Bibliotecas de terceiros leem arquivos não confiáveis fora de um ambiente isolado, e os servidores de processamento têm acesso amplo ao armazenamento|2|4|8|Alto|
|[T22], [CA05]|Elevation of Privilege: um cliente aprova a revisão técnica da própria peça ou acessa funções administrativas (RF19, RF20)|A permissão é verificada só na interface, que esconde a opção, e o servidor não confere o perfil nas rotas de aprovação e administração|3|4|12|Crítico|
|[TC01] ([Etapa 1, seção 4][e1-4])|Spoofing: uma confirmação de pagamento falsa libera o pedido para produção (RF08)|O pedido pode ser marcado como pago a partir do retorno do navegador ou de uma notificação cuja assinatura não é validada junto ao gateway|2|3|6|Moderado|
|[TC02] ([Etapa 1, seção 3.2][e1-32])|Elevation of Privilege: um script escondido em mensagem de suporte, avaliação ou nome de arquivo roda no painel interno (XSS armazenado) e rouba a sessão de um técnico ou administrador (RF11, RF13)|O painel exibe conteúdo enviado pelo cliente sem codificação de saída e sem Content Security Policy|2|4|8|Alto|
|[TC03] ([Etapa 1, seção 3.2][e1-32])|Tampering, com efeito de Repudiation: os logs de auditoria são alterados ou apagados por alguém da equipe ou por um processo comprometido|Os logs ficam na mesma base e com as mesmas permissões da aplicação, sem armazenamento que só aceite inclusões e sem cópia externa|2|3|6|Moderado|
|[TC04] ([Etapa 1, seção 3.1][e1-31])|Elevation of Privilege: um PDF malicioso (RF22) aberto pelo técnico compromete a máquina dele|Os PDFs dos clientes são abertos diretamente, sem conversão, limpeza ou visualização isolada|2|3|6|Moderado|

### 2.3 Distribuição na matriz P x I

|Probabilidade \ Impacto|Baixo (1)|Moderado (2)|Alto (3)|Muito alto (4)|
|-|-|-|-|-|
|**Alta (4)**|-|-|-|-|
|**Média-alta (3)**|-|[T08], [T11], [T18]|[T01], [T02], [T07], [T16], [T19]|**[T03], [T06], [T14], [T17], [T20], [T22]**|
|**Média-baixa (2)**|[T13]|[T04], [T10]|[T05], [T09], [T12], [T15], [TC01], [TC03], [TC04]|[T21], [TC02]|
|**Baixa (1)**|-|-|-|-|

São 26 riscos: 6 críticos, 7 altos, 12 moderados e 1 baixo.

## Parte 3 - Plano de priorização

### 3.1 Riscos críticos (12 a 16)

Os seis riscos críticos receberam probabilidade 3 e impacto 4. Eles se resumem a três causas, e cada uma depende de decisões de arquitetura. Se essas decisões ficarem para depois do código, a correção vai exigir mudanças em muitos pontos do sistema, e qualquer ponto esquecido mantém o risco.

**Grupo A: permissão por objeto ([T06], [T14] e [T17], fluxos [F2] e [F3])**

Os clientes enviam projetos que muitas vezes ainda não foram patenteados, e só fazem isso se confiarem no sigilo da plataforma. Um STL ou PDF vazado não pode ser recuperado. Como os números de pedido são sequenciais, um script simples consegue baixar todos os arquivos ([CA01]) ou cancelar e aprovar pedidos de todos os clientes ([CA06]).

Três decisões precisam ser tomadas antes de escrever os endpoints:

1. Usar identificadores aleatórios (UUID) para pedidos, arquivos e laudos. Trocar o formato depois exige migrar os dados e quebra links já enviados aos clientes.
2. Manter o armazenamento privado e liberar cada arquivo por uma URL pré-assinada de curta duração, gerada pela API depois de conferir o dono. Isso define como o visualizador 3D (RF03) carrega os modelos.
3. Fazer a verificação de permissão em um único ponto da API, negando tudo o que não for explicitamente permitido. Se cada rota fizer a própria verificação, basta uma rota esquecida para o problema continuar.

**Grupo B: painel interno e privilégios ([T03] e [T22], fluxo [F6])**

Quem controla o painel consegue ler os projetos de todos os clientes, aprovar peças sem revisão ([CA05], [CA08]) e mudar preços e descontos. O prejuízo costuma aparecer depois, como uma peça defeituosa entregue ao cliente.

A arquitetura precisa definir se as contas de clientes e de funcionários ficam no mesmo cadastro, se o painel tem endereço e rotas próprios e onde as permissões de cada perfil ([seção 3.1 da Etapa 1][e1-31]) são verificadas. A proposta é:

* separar o painel da aplicação do cliente e liberar o acesso apenas pela VPN ou por uma lista de IPs;
* exigir login único (SSO) com MFA para todos os perfis internos;
* verificar o perfil no servidor em toda rota de aprovação e administração, sem depender do que a interface mostra ([CA05]);
* documentar o controle de acesso da equipe interna: quem cria as contas, quem aprova cada perfil, quando os acessos são revisados e como são removidos quando alguém sai da empresa.

**Grupo C: disponibilidade do processamento ([T20], fluxo [F2])**

Renderização, fatiamento e DFM fazem parte de todo orçamento (RF03, RF05, RF06, RF19). Se esses servidores ficarem sobrecarregados, nenhum cliente consegue orçar ou comprar.

A escolha entre processar na hora ou por fila muda o contrato da API e a interface, porque o orçamento passa a ter um estado "em processamento". A proposta é usar uma fila com workers, cota por conta, limites de tamanho e de polígonos conferidos antes de a tarefa entrar na fila, tempo máximo e limite de memória por tarefa e escalonamento com teto de custo. O [T21] (Alto) afeta o mesmo componente, então o isolamento dos workers deve ser decidido junto.

### 3.2 Riscos altos e moderados

A ordem parte da pontuação, com alguns ajustes. Um risco resolvido pelo mesmo controle de um crítico vem logo depois dele. Um risco capaz de contornar controles já planejados sobe na lista. Medidas de processo, que podem ser adotadas a qualquer momento e sem código, descem.

**Riscos altos (8 a 11)**

|Ordem|Risco|Pontuação|Fluxo|Justificativa da posição|
|-:|-|-:|-|-|
|1|[T21]: execução de código por STL malicioso|8|[F2]|O impacto é o maior da escala: quem controla os workers alcança os arquivos de todos os clientes. A probabilidade ficou em 2 porque o ataque depende de uma falha no leitor de STL e de conhecimento técnico para explorá-la. Vem em primeiro porque está no mesmo componente do [T20] e deve ser tratado junto: workers isolados, sem acesso à rede, sem privilégios e com permissão apenas para o arquivo da tarefa.|
|2|[T16]: dados de outros clientes|9|[F3]|Tem a mesma causa do [T06] e do [T14] (IDOR, agora em consultas) e fica resolvido com a verificação de permissão do grupo A. Envolve CPF e endereço, que são protegidos pela LGPD.|
|3|[T07]: preço alterado no checkout|9|[F4]|Qualquer cliente com um proxy consegue repetir o ataque, e o prejuízo é direto. A correção é simples: no checkout o servidor recebe só os identificadores (material, quantidade e opções) e refaz o cálculo.|
|4|[T01]: conta de cliente invadida|9|[F1]|É a entrada para todos os fluxos do cliente, mas o dano fica limitado a uma conta por vez. Limite de tentativas, regras de sessão e MFA opcional resolvem a maior parte, e o limite de tentativas também atende o [T18].|
|5|[T19]: envio abusivo de arquivos|9|[F2]|Os limites de tamanho, formato e espaço por conta são conferidos na API antes de gravar o arquivo, no mesmo trabalho do grupo C.|
|6|[TC02]: XSS armazenado no painel|8|[F3] e [F6]|Fica à frente do [T02], mesmo com pontuação menor, porque o script roda numa sessão interna já autenticada e assim contorna a VPN e o MFA do grupo B. A codificação de saída e a CSP precisam existir desde a primeira versão do painel.|
|7|[T02]: desvio de entrega pelo suporte|9|[F6]|Afeta um pedido por vez e depende de dados da vítima. A correção é uma regra de atendimento (confirmar pelo canal cadastrado antes de trocar o endereço), que pode ser adotada a qualquer momento.|

**Riscos moderados (4 a 7)**

|Ordem|Risco|Pontuação|Fluxo|Justificativa da posição|
|-:|-|-:|-|-|
|1|[TC03]: logs de auditoria alterados|6|Todos|Sem logs confiáveis, [T10], [T11] e [T12] ficam sem solução e os incidentes críticos não podem ser investigados ([CA08]). Por isso vem antes dos outros moderados.|
|2|[T12]: técnico nega a aprovação|6|[F6]|Depende do grupo B: cada aprovação é feita com conta pessoal e fica registrada no log de auditoria.|
|3|[TC01]: pagamento falso|6|[F4]|O prejuízo é direto e a correção é simples: o pedido só é marcado como pago depois de consultar o gateway ou validar a assinatura da notificação.|
|4|[T08]: desconto indevido|6|[F4]|Fica resolvido com o mesmo recálculo no servidor do [T07].|
|5|[T15]: dados de pagamento expostos|6|[F4]|Se o pagamento usar a página ou a tokenização do próprio gateway, o número do cartão nem passa pelo sistema. O restante é mascarar dados nos logs e restringir a API de checkout.|
|6|[T05]: STL alterado|6|[F2]|TLS no envio e hash SHA-256 calculado no upload e conferido antes do fatiamento e da impressão.|
|7|[T09]: laudo DFM alterado|6|[F6]|Depende de quem pode editar o laudo (grupo B). Resolve-se guardando as versões do laudo e o hash da versão enviada ao cliente.|
|8|[TC04]: PDF malicioso|6|[F2] e [F6]|Usa o mesmo ambiente isolado do [T21]: o PDF é convertido em imagem dentro do worker e o técnico não abre o arquivo original.|
|9|[T11]: cliente nega o cancelamento|6|[F3]|Resolvido pelo log de auditoria ([TC03]) e por uma confirmação enviada ao canal cadastrado ([F7]).|
|10|[T18]: bloqueio por força bruta|6|[F1]|Usa o limite de tentativas do [T01]. É melhor aumentar o tempo de espera ou pedir CAPTCHA do que bloquear a conta, porque o bloqueio pode ser usado para impedir o acesso do próprio dono.|
|11|[T04]: transportadora falsa|4|[F5]|Autenticar o retorno de rastreio (HMAC ou lista de IPs) ou consultar a API da transportadora. O dano pode ser revertido.|
|12|[T10]: disputa de pagamento|4|[F4]|O gateway guarda os comprovantes. Basta vincular o ID da transação ao pedido e conferir os dois periodicamente.|

**Risco baixo (1 a 3):** [T13], com pontuação 2. A Etapa 1 não considera as avaliações um ativo importante ([seção 3.2][e1-32]). A proposta é aceitar o risco, registrando a conta que enviou cada avaliação, e reavaliar caso as avaliações passem a influenciar as vendas.

### 3.3 Recomendações próximas

As medidas abaixo estão agrupadas por área. As três primeiras cobrem todos os riscos críticos e devem entrar na Etapa 3 (arquitetura segura).

|Área|Medidas imediatas|Riscos reduzidos|
|-|-|-|
|Controle de acesso|Verificação de permissão em um único ponto da API, negando por padrão; conferência do dono em todo acesso a pedido, arquivo e laudo; IDs aleatórios (UUID); permissões dos quatro perfis descritos na [seção 3.1 da Etapa 1][e1-31] verificadas no servidor|[T06], [T14], [T16], [T17], [T22]|
|Autenticação|SSO com MFA para perfis internos; painel em endereço separado, acessível pela VPN ou por lista de IPs; processo documentado de criação, revisão e remoção de contas internas; limite de tentativas com espera crescente; sessões com expiração e cookies `HttpOnly`, `Secure` e `SameSite`|[T01], [T03], [T18], [TC02]|
|Processamento isolado|Fila com cota por conta; limites de tamanho e de polígonos conferidos na API; workers descartáveis em contêiner sem rede e sem privilégios, com tempo máximo e limite de memória; credencial do worker válida só para o arquivo da tarefa|[T19], [T20], [T21], [TC04]|
|Regras de negócio no servidor|Preço, desconto e frete recalculados no checkout; pagamento confirmado com o gateway ou pela assinatura da notificação; retorno da transportadora autenticado|[T04], [T07], [T08], [TC01]|
|Criptografia e proteção de dados|TLS em todos os fluxos ([F1] a [F7]); criptografia em repouso no armazenamento e no banco; hash SHA-256 do arquivo no upload, conferido antes do fatiamento e da impressão; dados pessoais e de pagamento mascarados nos logs; tokenização pelo gateway, sem guardar número de cartão|[T05], [T09], [T15], e reforço para [T14] e [T17]|
|Registro de auditoria|Log que só aceita inclusões, guardado fora da base da aplicação, com quem fez, quando, o quê e os valores antes e depois, para aprovações, cancelamentos, pagamentos, trocas de endereço e laudos|[T10], [T11], [T12], [T13], [TC03]|
|Atendimento|Troca de endereço só depois de confirmação pelo e-mail ou telefone cadastrado; nenhuma troca depois do despacho|[T02]|

[e1-31]: etapa-1-ameacas-stride.md#31-usuários-e-perfis-de-acesso
[e1-32]: etapa-1-ameacas-stride.md#32-elementos-do-sistema
[e1-4]: etapa-1-ameacas-stride.md#4-visão-geral-da-arquitetura-e-fluxo

[T01]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T02]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T03]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T04]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T05]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T06]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T07]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T08]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T09]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T10]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T11]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T12]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T13]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T14]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T15]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T16]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T17]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T18]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T19]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T20]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T21]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride
[T22]: etapa-1-ameacas-stride.md#5-modelagem-de-ameaças-com-stride

[CA01]: etapa-1-ameacas-stride.md#ca01--roubo-de-projeto-confidencial
[CA02]: etapa-1-ameacas-stride.md#ca02--arquivo-stl-malicioso
[CA03]: etapa-1-ameacas-stride.md#ca03--alteração-do-valor-do-pedido
[CA04]: etapa-1-ameacas-stride.md#ca04--desvio-de-entrega-pelo-suporte
[CA05]: etapa-1-ameacas-stride.md#ca05--autoaprovação-da-revisão-técnica
[CA06]: etapa-1-ameacas-stride.md#ca06--cancelamento-de-pedidos-de-outros-clientes
[CA07]: etapa-1-ameacas-stride.md#ca07--sobrecarga-do-processamento-de-modelos
[CA08]: etapa-1-ameacas-stride.md#ca08--falso-técnico-aprova-peça-adulterada

[F1]: #12-fluxos-de-dados
[F2]: #12-fluxos-de-dados
[F3]: #12-fluxos-de-dados
[F4]: #12-fluxos-de-dados
[F5]: #12-fluxos-de-dados
[F6]: #12-fluxos-de-dados
[F7]: #12-fluxos-de-dados

[TC01]: #14-ameaças-complementares
[TC02]: #14-ameaças-complementares
[TC03]: #14-ameaças-complementares
[TC04]: #14-ameaças-complementares
