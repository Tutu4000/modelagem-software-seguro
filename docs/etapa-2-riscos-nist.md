# Etapa 2 — Análise, Priorização e Tratamento de Riscos com o NIST CSF 2.0

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

Ao percorrer os fluxos, encontramos quatro ameaças que não constam da Etapa 1. Elas foram registradas na Parte 2 como TC01 a TC04 (riscos R23 a R26):

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
|4 a 7|Médio|
|8 a 11|Alto|
|12 a 16|Crítico|

As notas partem de duas premissas:

* O sistema ainda está em projeto e a Etapa 1 não diz quais controles existem. As condições descritas nos casos de abuso foram tratadas como plausíveis, mas não como certas, por isso nenhuma ameaça recebeu probabilidade 4.
* Vazamento de dados pessoais recebeu impacto 3, como a escala indica. O impacto 4 ficou para eventos irreversíveis ou que atingem todos os clientes de uma vez: vazamento de projetos, alteração de pedidos em massa, parada do processamento e controle do painel interno.

### 2.2 Registro de riscos

Cada linha é um risco. O ID segue a numeração da ameaça de origem: R01 a R22 correspondem a T01 a T22, e R23 a R26 correspondem às ameaças complementares TC01 a TC04.

|ID|Origem STRIDE|Evento de risco|Vulnerabilidade ou condição|Probabilidade|Impacto|Pontuação|Nível|
|-|-|-|-|-:|-:|-:|-|
|R01|Spoofing ([T01])|Alguém entra na conta do cliente com credenciais vazadas ou sessão sequestrada (RF01)|A arquitetura não prevê MFA, limite de tentativas de login, alerta de acesso suspeito nem regras de sessão (expiração, troca de token, cookies `HttpOnly` e `Secure`)|3|3|9|Alto|
|R02|Spoofing ([T02], [CA04])|Um golpista se passa pelo cliente no suporte e troca o endereço de entrega (RF11)|O suporte confirma a identidade só com nome, CPF e número do pedido, que são dados fáceis de conseguir, e não confirma a troca pelo canal cadastrado|3|3|9|Alto|
|R03|Spoofing ([T03], [CA08])|Um atacante usa as credenciais de um técnico para entrar no painel interno (RF20)|O painel não exige MFA nem restringe a rede de acesso, e o processo de controle de acesso dos administradores e da equipe interna (criação, revisão e remoção de contas) não está documentado|3|4|12|Crítico|
|R04|Spoofing ([T04])|Alguém se passa pela transportadora e envia valores de frete ou status de entrega falsos (RF07)|O retorno de rastreio da transportadora para a API não tem autenticação de origem (assinatura HMAC, mTLS ou lista de IPs)|2|2|4|Médio|
|R05|Tampering ([T05], [CA02])|O STL é alterado no envio ou no armazenamento antes do fatiamento (RF02)|Não há conferência de integridade (hash) entre upload, armazenamento e produção, e o uso de TLS não está especificado|2|3|6|Médio|
|R06|Tampering ([T06], [CA06])|Um cliente cancela pedidos de outros clientes ou altera o status deles (RF10, RF12, RF20)|Os números de pedido são sequenciais e o servidor não confere se o pedido pertence a quem fez a requisição (IDOR)|3|4|12|Crítico|
|R07|Tampering ([T07], [CA03])|O cliente altera o preço na requisição de checkout (RF06, RF23)|O servidor pode aceitar o preço, o material ou a quantidade enviados pelo navegador sem refazer o cálculo|3|3|9|Alto|
|R08|Tampering ([T08], [CA03])|O cliente informa uma quantidade falsa para ganhar o desconto progressivo (RF24)|O desconto é calculado com parâmetros enviados pelo cliente, sem conferir a quantidade efetivamente paga|3|2|6|Médio|
|R09|Tampering ([T09], [CA08])|O laudo DFM é alterado antes de chegar ao cliente (RF19, RF20)|O laudo fica no armazenamento sem conferência de integridade nem histórico de versões, e as edições não registram o autor|2|3|6|Médio|
|R10|Repudiation ([T10])|O cliente contesta um pagamento e não há registro que comprove a transação (RF08)|O ID da transação no gateway não é vinculado ao pedido e não há conciliação|2|2|4|Médio|
|R11|Repudiation ([T11], [CA06])|O cliente nega ter pedido o cancelamento (RF10)|O cancelamento não registra quem o fez (usuário, data e IP) e não gera confirmação no canal cadastrado|3|2|6|Médio|
|R12|Repudiation ([T12], [CA08])|O técnico nega ter aprovado um projeto com falha (RF20)|As aprovações não ficam registradas de forma nominal e imutável, e a produção pode usar contas compartilhadas|2|3|6|Médio|
|R13|Repudiation ([T13])|O usuário nega ter escrito uma avaliação ofensiva (RF13)|A avaliação não fica registrada junto à conta e à sessão que a enviaram|2|1|2|Baixo|
|R14|Information Disclosure ([T14], [CA01])|Um usuário baixa STL de outros clientes trocando o identificador na URL (RF02, RF03)|Os arquivos são acessados por URL previsível ou pública, sem URL pré-assinada e sem conferência do dono|3|4|12|Crítico|
|R15|Information Disclosure ([T15])|Dados de pagamento aparecem em logs ou na API de checkout (RF08)|Não há regra de mascaramento nos logs, e as permissões da API de checkout não foram definidas|2|3|6|Médio|
|R16|Information Disclosure ([T16])|Um usuário consulta histórico, cadastro (CPF e endereço) ou laudos de outros clientes (RF09, RF10, RF20)|O mesmo IDOR do [R06], agora em consultas, com IDs sequenciais|3|3|9|Alto|
|R17|Information Disclosure ([T17], [CA01])|Desenhos técnicos em PDF, com segredos industriais, são acessados sem autorização (RF22)|Os PDFs ficam no mesmo armazenamento do [R14], sem separação por cliente, e os técnicos têm acesso a todos os arquivos|3|4|12|Crítico|
|R18|Denial of Service ([T18])|Tentativas de força bruta no login bloqueiam usuários legítimos (RF01)|Não há limite de tentativas por IP ou por conta, e um bloqueio de conta mal desenhado pode ser usado para impedir o acesso de terceiros|3|2|6|Médio|
|R19|Denial of Service ([T19], [CA07])|Envio em massa de arquivos, zip bombs e modelos gigantes esgotam o armazenamento e o processamento (RF02)|Não há limite de tamanho, de envios por conta nem de espaço, e o formato não é validado antes da gravação|3|3|9|Alto|
|R20|Denial of Service ([T20], [CA07])|Muitos modelos de alta densidade enviados ao mesmo tempo sobrecarregam renderização, fatiamento e DFM (RF03, RF05, RF06, RF19)|O processamento não tem fila, cota por usuário, limite de polígonos nem tempo máximo por tarefa|3|4|12|Crítico|
|R21|Elevation of Privilege ([T21], [CA02])|Um STL malicioso explora uma falha do fatiador ou do visualizador e executa código no servidor (RF02, RF03)|Bibliotecas de terceiros leem arquivos não confiáveis fora de um ambiente isolado, e os servidores de processamento têm acesso amplo ao armazenamento|2|4|8|Alto|
|R22|Elevation of Privilege ([T22], [CA05])|Um cliente aprova a revisão técnica da própria peça ou acessa funções administrativas (RF19, RF20)|A permissão é verificada só na interface, que esconde a opção, e o servidor não confere o perfil nas rotas de aprovação e administração|3|4|12|Crítico|
|R23|Spoofing ([TC01], [Etapa 1, seção 4][e1-4])|Uma confirmação de pagamento falsa libera o pedido para produção (RF08)|O pedido pode ser marcado como pago a partir do retorno do navegador ou de uma notificação cuja assinatura não é validada junto ao gateway|2|3|6|Médio|
|R24|Elevation of Privilege ([TC02], [Etapa 1, seção 3.2][e1-32])|Um script escondido em mensagem de suporte, avaliação ou nome de arquivo roda no painel interno (XSS armazenado) e rouba a sessão de um técnico ou administrador (RF11, RF13)|O painel exibe conteúdo enviado pelo cliente sem codificação de saída e sem Content Security Policy|2|4|8|Alto|
|R25|Tampering, com efeito de Repudiation ([TC03], [Etapa 1, seção 3.2][e1-32])|Os logs de auditoria são alterados ou apagados por alguém da equipe ou por um processo comprometido|Os logs ficam na mesma base e com as mesmas permissões da aplicação, sem armazenamento que só aceite inclusões e sem cópia externa|2|3|6|Médio|
|R26|Elevation of Privilege ([TC04], [Etapa 1, seção 3.1][e1-31])|Um PDF malicioso (RF22) aberto pelo técnico compromete a máquina dele|Os PDFs dos clientes são abertos diretamente, sem conversão, limpeza ou visualização isolada|2|3|6|Médio|

### 2.3 Distribuição na matriz P x I

|Probabilidade \ Impacto|Baixo (1)|Moderado (2)|Alto (3)|Muito alto (4)|
|-|-|-|-|-|
|**Alta (4)**|-|-|-|-|
|**Média-alta (3)**|-|[R08], [R11], [R18]|[R01], [R02], [R07], [R16], [R19]|**[R03], [R06], [R14], [R17], [R20], [R22]**|
|**Média-baixa (2)**|[R13]|[R04], [R10]|[R05], [R09], [R12], [R15], [R23], [R25], [R26]|[R21], [R24]|
|**Baixa (1)**|-|-|-|-|

São 26 riscos: 6 críticos, 7 altos, 12 médios e 1 baixo.

### 2.4 Justificativas das avaliações

A probabilidade 3 foi dada quando o ataque pode ser feito com ferramentas comuns (navegador, proxy, script ou listas de senhas vazadas) e a condição descrita na Etapa 1 é plausível. A probabilidade 2 foi dada quando o ataque depende de uma falha específica de configuração ou de implementação, de capacidade técnica ou de acesso interno.

|Risco|Por que a probabilidade|Por que o impacto|Afetados e consequências|
|-|-|-|-|
|[R01]|**3:** ataques com senhas vazadas são comuns e automatizados, e o login não tem MFA nem limite de tentativas|**3:** o dano fica em uma conta por vez, mas inclui os projetos e os pedidos daquele cliente|Cliente e seus STL, PDF, dados pessoais e pedidos. Vazamento de projetos de uma conta e pedidos fraudulentos|
|[R02]|**3:** nome, CPF e número do pedido aparecem em notas fiscais, e-mails e vazamentos, e o suporte só confere esses dados|**3:** a vítima perde uma peça já paga e a empresa precisa reembolsar|Cliente, suporte e financeiro. Peça desviada, reembolso e perda de confiança|
|[R03]|**3:** técnicos usam o painel todos os dias, phishing direcionado é comum e contas de ex-funcionários podem continuar ativas ([CA08])|**4:** o painel dá acesso aos projetos de todos os clientes e às aprovações, e um vazamento de projetos não pode ser desfeito|Todos os clientes e a produção. Vazamento em massa, aprovações indevidas e peças defeituosas|
|[R04]|**2:** o atacante precisa descobrir o endereço e o formato do retorno da transportadora, que não ficam visíveis ao cliente|**2:** o status ou o frete errado é corrigido consultando a transportadora, sem perda de dados|Clientes com pedidos em trânsito e suporte. Informação de entrega errada e mais chamados|
|[R05]|**2:** alterar o arquivo exige interceptar o tráfego ou ter permissão de escrita no armazenamento|**3:** uma peça com defeito oculto pode ser fabricada, entregue e falhar durante o uso|Cliente e produção. Peça defeituosa, retrabalho e discussão de responsabilidade|
|[R06]|**3:** com IDs sequenciais e sem conferência do dono, basta trocar um número na requisição, e um script repete isso para todos os pedidos ([CA06])|**4:** pode cancelar ou aprovar todos os pedidos em produção ao mesmo tempo|Todos os clientes, produção e financeiro. Atrasos em massa, aprovações indevidas e reembolsos|
|[R07]|**3:** qualquer cliente consegue alterar a requisição com as ferramentas do navegador ou um proxy|**3:** prejuízo financeiro direto em cada pedido adulterado|Empresa (financeiro). Peças vendidas abaixo do custo|
|[R08]|**3:** a mesma facilidade do [R07], porque a quantidade vem do cliente|**2:** o prejuízo se limita ao percentual do desconto e pode ser cobrado depois da conferência do pedido|Empresa (financeiro). Descontos indevidos|
|[R09]|**2:** é preciso ter permissão de escrita sobre o laudo, por meio de uma conta interna ([R03]) ou de uma falha no armazenamento|**3:** o cliente muda o projeto seguindo uma orientação falsa e recebe uma peça defeituosa|Cliente e técnicos. Peça defeituosa e perda de confiança no laudo|
|[R10]|**2:** contestações acontecem, mas o gateway guarda os comprovantes. O problema só aparece se a transação não estiver ligada ao pedido|**2:** a disputa envolve um pedido e pode ser resolvida com o comprovante do gateway|Cliente e financeiro. Atraso na solução da disputa|
|[R11]|**3:** arrependimento e contestação de cancelamento são comuns no pós-venda, e hoje não há log nem confirmação|**2:** a disputa envolve o reembolso de um pedido e tem solução|Cliente, suporte e financeiro. Reembolso contestado sem evidência|
|[R12]|**2:** depende de uma peça aprovada com falha e do uso de contas compartilhadas|**3:** sem saber quem aprovou, a empresa não consegue apurar a responsabilidade pela peça defeituosa|Técnicos, produção e cliente. Responsabilidade indefinida e risco jurídico|
|[R13]|**2:** avaliações ofensivas acontecem, mas negar a autoria exige que a avaliação não esteja ligada a uma conta|**1:** avaliações não são ativo importante ([seção 3.2 da Etapa 1][e1-32]) e podem ser removidas|Empresa (reputação). Pequeno transtorno|
|[R14]|**3:** basta copiar o endereço do próprio arquivo no visualizador e trocar o identificador ([CA01])|**4:** expõe a propriedade intelectual de todos os clientes, e o vazamento não pode ser desfeito|Todos os clientes. Vazamento de projetos e perda de confiança na plataforma|
|[R15]|**2:** depende de um erro de configuração, como um log de depuração ou uma permissão mal definida na API de checkout|**3:** dados de pagamento expostos permitem fraude e ferem a LGPD|Clientes e financeiro. Fraude, notificação à ANPD e multa|
|[R16]|**3:** o mesmo IDOR do [R06], com a mesma facilidade|**3:** vazamento de dados pessoais (CPF e endereço) e laudos, como define a premissa da [seção 2.1](#21-critérios)|Clientes. Violação de privacidade, LGPD e golpes usando os dados vazados|
|[R17]|**3:** os PDFs ficam no mesmo armazenamento do [R14] e todos os técnicos acessam todos os arquivos|**4:** os desenhos têm segredos industriais, e o vazamento não pode ser desfeito|Clientes que usam o RF22. Vazamento de propriedade intelectual|
|[R18]|**3:** força bruta automatizada contra telas de login é frequente em qualquer serviço exposto|**2:** as contas atingidas ficam sem acesso por um tempo, sem perda de dados|Clientes. Indisponibilidade temporária do login|
|[R19]|**3:** qualquer conta pode enviar arquivos sem limite, e gerar um arquivo enorme é trivial|**3:** o serviço fica lento e o custo de armazenamento sobe, mas os arquivos podem ser apagados e o serviço volta sem perda de dados|Todos os clientes e infraestrutura. Lentidão e custo extra|
|[R20]|**3:** até clientes legítimos enviam modelos pesados, e um atacante com várias contas ([CA07]) satura o processamento sem fila nem cota|**4:** sem processamento, nenhum cliente consegue orçar ou comprar|Todos os clientes e a receita. Parada do serviço principal|
|[R21]|**2:** exige uma falha conhecida no leitor de STL e capacidade técnica para explorá-la|**4:** quem controla os servidores alcança os arquivos de todos os clientes|Todos os clientes e infraestrutura. Comprometimento dos servidores e vazamento em massa|
|[R22]|**3:** a requisição de aprovação aparece no navegador, e a interface só esconde a opção ([CA05])|**4:** permite aprovar peças sem revisão e usar funções administrativas como preços, descontos e contas|Produção, todos os clientes e financeiro. Aprovações indevidas e alteração de preços|
|[R23]|**2:** depende de a integração aceitar o retorno do navegador ou não validar a assinatura da notificação|**3:** a peça é produzida e entregue sem pagamento|Financeiro e produção. Prejuízo por pedido|
|[R24]|**2:** exige conhecimento de XSS e que o painel não codifique a saída|**4:** a sessão roubada de um técnico ou administrador tem o mesmo alcance do [R03]|Equipe interna e todos os clientes. Vazamento em massa e aprovações indevidas|
|[R25]|**2:** exige acesso interno ou um processo comprometido com permissão sobre a base|**3:** as evidências de disputas e incidentes se perdem, o que agrava [R10], [R11] e [R12]|Empresa e clientes em disputa. Responsabilidades e incidentes sem apuração|
|[R26]|**2:** depende de uma falha no leitor de PDF do técnico e de um ataque direcionado|**3:** compromete a máquina de um técnico, que tem acesso ao painel, mas o dano começa em uma estação|Técnicos e painel interno. Porta de entrada para o [R03]|

**Por que os níveis representam o contexto:**

* **Críticos:** todos combinam um ataque fácil (P = 3) com um dano irreversível ou que atinge todos os clientes (I = 4). São os riscos que a plataforma não consegue absorver, porque perder o sigilo dos projetos encerra o modelo de negócio.
* **Altos:** são de dois tipos. Uns são fáceis, mas atingem um cliente ou um pedido por vez ([R01], [R02], [R07], [R16], [R19]). Outros são graves, mas dependem de capacidade técnica ([R21], [R24]).
* **Médios:** dependem de condições específicas ou causam danos que podem ser recuperados.
* **Baixo:** só o [R13], cujo ativo não foi considerado importante na Etapa 1.

A pontuação serviu de ponto de partida. A Parte 3 ajusta a ordem entre riscos com a mesma pontuação.

## Parte 3 - Plano de priorização

### 3.1 Riscos críticos (12 a 16)

Os seis riscos críticos receberam probabilidade 3 e impacto 4. Eles se resumem a três causas, e cada uma depende de decisões de arquitetura. Se essas decisões ficarem para depois do código, a correção vai exigir mudanças em muitos pontos do sistema, e qualquer ponto esquecido mantém o risco.

**Grupo A: permissão por objeto ([R06], [R14] e [R17], fluxos [F2] e [F3])**

Os clientes enviam projetos que muitas vezes ainda não foram patenteados, e só fazem isso se confiarem no sigilo da plataforma. Um STL ou PDF vazado não pode ser recuperado. Como os números de pedido são sequenciais, um script simples consegue baixar todos os arquivos ([CA01]) ou cancelar e aprovar pedidos de todos os clientes ([CA06]).

Três decisões precisam ser tomadas antes de escrever os endpoints:

1. Usar identificadores aleatórios (UUID) para pedidos, arquivos e laudos. Trocar o formato depois exige migrar os dados e quebra links já enviados aos clientes.
2. Manter o armazenamento privado e liberar cada arquivo por uma URL pré-assinada de curta duração, gerada pela API depois de conferir o dono. Isso define como o visualizador 3D (RF03) carrega os modelos.
3. Fazer a verificação de permissão em um único ponto da API, negando tudo o que não for explicitamente permitido. Se cada rota fizer a própria verificação, basta uma rota esquecida para o problema continuar.

**Grupo B: painel interno e privilégios ([R03] e [R22], fluxo [F6])**

Quem controla o painel consegue ler os projetos de todos os clientes, aprovar peças sem revisão ([CA05], [CA08]) e mudar preços e descontos. O prejuízo costuma aparecer depois, como uma peça defeituosa entregue ao cliente.

A arquitetura precisa definir se as contas de clientes e de funcionários ficam no mesmo cadastro, se o painel tem endereço e rotas próprios e onde as permissões de cada perfil ([seção 3.1 da Etapa 1][e1-31]) são verificadas. A proposta é:

* separar o painel da aplicação do cliente e liberar o acesso apenas pela VPN ou por uma lista de IPs;
* exigir login único (SSO) com MFA para todos os perfis internos;
* verificar o perfil no servidor em toda rota de aprovação e administração, sem depender do que a interface mostra ([CA05]);
* documentar o controle de acesso da equipe interna: quem cria as contas, quem aprova cada perfil, quando os acessos são revisados e como são removidos quando alguém sai da empresa.

**Grupo C: disponibilidade do processamento ([R20], fluxo [F2])**

Renderização, fatiamento e DFM fazem parte de todo orçamento (RF03, RF05, RF06, RF19). Se esses servidores ficarem sobrecarregados, nenhum cliente consegue orçar ou comprar.

A escolha entre processar na hora ou por fila muda o contrato da API e a interface, porque o orçamento passa a ter um estado "em processamento". A proposta é usar uma fila com workers, cota por conta, limites de tamanho e de polígonos conferidos antes de a tarefa entrar na fila, tempo máximo e limite de memória por tarefa e escalonamento com teto de custo. O [R21] (Alto) afeta o mesmo componente, então o isolamento dos workers deve ser decidido junto.

### 3.2 Riscos altos e médios

A ordem parte da pontuação, com alguns ajustes. Um risco resolvido pelo mesmo controle de um crítico vem logo depois dele. Um risco capaz de contornar controles já planejados sobe na lista. Medidas de processo, que podem ser adotadas a qualquer momento e sem código, descem.

**Riscos altos (8 a 11)**

|Ordem|Risco|Pontuação|Fluxo|Justificativa da posição|
|-:|-|-:|-|-|
|1|[R21]: execução de código por STL malicioso|8|[F2]|O impacto é o maior da escala: quem controla os workers alcança os arquivos de todos os clientes. A probabilidade ficou em 2 porque o ataque depende de uma falha no leitor de STL e de conhecimento técnico para explorá-la. Vem em primeiro porque está no mesmo componente do [R20] e deve ser tratado junto: workers isolados, sem acesso à rede, sem privilégios e com permissão apenas para o arquivo da tarefa.|
|2|[R16]: dados de outros clientes|9|[F3]|Tem a mesma causa do [R06] e do [R14] (IDOR, agora em consultas) e fica resolvido com a verificação de permissão do grupo A. Envolve CPF e endereço, que são protegidos pela LGPD.|
|3|[R07]: preço alterado no checkout|9|[F4]|Qualquer cliente com um proxy consegue repetir o ataque, e o prejuízo é direto. A correção é simples: no checkout o servidor recebe só os identificadores (material, quantidade e opções) e refaz o cálculo.|
|4|[R01]: conta de cliente invadida|9|[F1]|É a entrada para todos os fluxos do cliente, mas o dano fica limitado a uma conta por vez. Limite de tentativas, regras de sessão e MFA opcional resolvem a maior parte, e o limite de tentativas também atende o [R18].|
|5|[R19]: envio abusivo de arquivos|9|[F2]|Os limites de tamanho, formato e espaço por conta são conferidos na API antes de gravar o arquivo, no mesmo trabalho do grupo C.|
|6|[R24]: XSS armazenado no painel|8|[F3] e [F6]|Fica à frente do [R02], mesmo com pontuação menor, porque o script roda numa sessão interna já autenticada e assim contorna a VPN e o MFA do grupo B. A codificação de saída e a CSP precisam existir desde a primeira versão do painel.|
|7|[R02]: desvio de entrega pelo suporte|9|[F6]|Afeta um pedido por vez e depende de dados da vítima. A correção é uma regra de atendimento (confirmar pelo canal cadastrado antes de trocar o endereço), que pode ser adotada a qualquer momento.|

**Riscos médios (4 a 7)**

|Ordem|Risco|Pontuação|Fluxo|Justificativa da posição|
|-:|-|-:|-|-|
|1|[R25]: logs de auditoria alterados|6|Todos|Sem logs confiáveis, [R10], [R11] e [R12] ficam sem solução e os incidentes críticos não podem ser investigados ([CA08]). Por isso vem antes dos outros médios.|
|2|[R12]: técnico nega a aprovação|6|[F6]|Depende do grupo B: cada aprovação é feita com conta pessoal e fica registrada no log de auditoria.|
|3|[R23]: pagamento falso|6|[F4]|O prejuízo é direto e a correção é simples: o pedido só é marcado como pago depois de consultar o gateway ou validar a assinatura da notificação.|
|4|[R08]: desconto indevido|6|[F4]|Fica resolvido com o mesmo recálculo no servidor do [R07].|
|5|[R15]: dados de pagamento expostos|6|[F4]|Se o pagamento usar a página ou a tokenização do próprio gateway, o número do cartão nem passa pelo sistema. O restante é mascarar dados nos logs e restringir a API de checkout.|
|6|[R05]: STL alterado|6|[F2]|TLS no envio e hash SHA-256 calculado no upload e conferido antes do fatiamento e da impressão.|
|7|[R09]: laudo DFM alterado|6|[F6]|Depende de quem pode editar o laudo (grupo B). Resolve-se guardando as versões do laudo e o hash da versão enviada ao cliente.|
|8|[R26]: PDF malicioso|6|[F2] e [F6]|Usa o mesmo ambiente isolado do [R21]: o PDF é convertido em imagem dentro do worker e o técnico não abre o arquivo original.|
|9|[R11]: cliente nega o cancelamento|6|[F3]|Resolvido pelo log de auditoria ([R25]) e por uma confirmação enviada ao canal cadastrado ([F7]).|
|10|[R18]: bloqueio por força bruta|6|[F1]|Usa o limite de tentativas do [R01]. É melhor aumentar o tempo de espera ou pedir CAPTCHA do que bloquear a conta, porque o bloqueio pode ser usado para impedir o acesso do próprio dono.|
|11|[R04]: transportadora falsa|4|[F5]|Autenticar o retorno de rastreio (HMAC ou lista de IPs) ou consultar a API da transportadora. O dano pode ser revertido.|
|12|[R10]: disputa de pagamento|4|[F4]|O gateway guarda os comprovantes. Basta vincular o ID da transação ao pedido e conferir os dois periodicamente.|

**Risco baixo (1 a 3):** [R13], com pontuação 2. A Etapa 1 não considera as avaliações um ativo importante ([seção 3.2][e1-32]). A proposta é aceitar o risco, registrando a conta que enviou cada avaliação, e reavaliar caso as avaliações passem a influenciar as vendas.

### 3.3 Recomendações próximas

As medidas foram agrupadas por área e detalhadas como controles no [catálogo da seção 4.3](#43-catálogo-de-controles), cada um com responsável e forma de verificação. As três primeiras áreas cobrem todos os riscos críticos e devem entrar na Etapa 3 (arquitetura segura).

|Área|Controles|Riscos reduzidos|
|-|-|-|
|Controle de acesso|C01 a C04|[R06], [R09], [R14], [R16], [R17], [R22]|
|Autenticação, sessões e painel interno|C05 a C09|[R01], [R03], [R12], [R18], [R24]|
|Processamento isolado|C10 a C12|[R19], [R20], [R21], [R26]|
|Regras de negócio e integrações|C13 a C15|[R04], [R07], [R08], [R10], [R23]|
|Criptografia e proteção de dados|C16 e C17|[R05], [R09], [R10], [R14], [R15], [R17]|
|Auditoria, detecção e recuperação|C18 a C20|Apoiam 17 riscos e são os controles principais do [R11] e do [R25]|
|Atendimento e governança|C21 e C22|[R02], [R13]|

## Parte 4 - Tratamento dos riscos

### 4.1 Estratégias de tratamento

|Estratégia|Uso neste sistema|
|-|-|
|Evitar|Eliminar a atividade ou condição que dá origem ao risco|
|Reduzir|Diminuir a probabilidade ou o impacto com controles|
|Compartilhar|Passar parte da operação ou das consequências a um terceiro|
|Aceitar|Manter o risco de forma consciente, com justificativa e acompanhamento|

|Risco|Estratégia|Justificativa|
|-|-|-|
|[R01]|Reduzir|O login é indispensável. Limite de tentativas, regras de sessão e MFA opcional diminuem a chance de invasão|
|[R02]|Reduzir|O suporte precisa continuar trocando endereços. Confirmar pelo canal cadastrado diminui a chance de golpe, e proibir a troca depois do despacho limita o dano|
|[R03]|Reduzir|O painel é necessário à operação. SSO com MFA, rede restrita e gestão das contas internas diminuem a chance de acesso indevido|
|[R04]|Reduzir|A integração com a transportadora é necessária. Autenticar o retorno impede mensagens de origem falsa|
|[R05]|Reduzir|O hash conferido antes do fatiamento detecta a alteração, e o TLS dificulta a interceptação|
|[R06]|Reduzir|A conferência do dono em um ponto único da API e os IDs aleatórios diminuem a chance do ataque|
|[R07]|Reduzir|O servidor recalcula o preço e ignora o valor enviado pelo navegador|
|[R08]|Reduzir|O mesmo recálculo do [R07] vale para o desconto|
|[R09]|Reduzir|Controle de quem pode editar, histórico de versões e hash da versão enviada ao cliente|
|[R10]|Compartilhar|O gateway guarda os comprovantes e conduz as contestações (chargeback) conforme o contrato. Ao sistema cabe vincular a transação ao pedido e conferir os dois|
|[R11]|Reduzir|O log de auditoria e a confirmação no canal cadastrado criam a prova do cancelamento|
|[R12]|Reduzir|Contas pessoais e log de auditoria tornam cada aprovação nominal|
|[R13]|Aceitar|O ativo não é importante e o impacto é 1. Veja as condições abaixo|
|[R14]|Reduzir|Armazenamento privado, URL pré-assinada e conferência do dono|
|[R15]|Evitar|Com a página ou a tokenização do gateway, o número do cartão nunca entra no sistema. O que sobra (valor, status e token) é mascarado nos logs|
|[R16]|Reduzir|A mesma conferência do dono do [R06], aplicada às consultas|
|[R17]|Reduzir|Os mesmos controles do [R14], mais a restrição dos técnicos aos arquivos dos pedidos atribuídos a eles|
|[R18]|Reduzir|Espera crescente e CAPTCHA em vez de bloqueio permanente da conta|
|[R19]|Reduzir|Limites de tamanho, formato e espaço conferidos antes de gravar o arquivo|
|[R20]|Reduzir|Fila, cota por conta e limites por tarefa fazem o sistema ficar mais lento em vez de parar|
|[R21]|Reduzir|Atualizar as bibliotecas diminui a probabilidade, e o isolamento dos workers diminui o impacto|
|[R22]|Reduzir|O perfil é conferido no servidor em todas as rotas de aprovação e administração|
|[R23]|Reduzir|O pedido só é marcado como pago depois de consultar o gateway ou validar a assinatura da notificação|
|[R24]|Reduzir|Codificação de saída e CSP impedem que o script rode no painel|
|[R25]|Reduzir|Log que só aceita inclusões, guardado fora da base e das permissões da aplicação|
|[R26]|Evitar|O técnico deixa de abrir o PDF original. O arquivo é convertido em imagem dentro do worker isolado do [R21], e só a imagem chega ao painel|

**Aceitação do [R13]**

* **Motivo:** a Etapa 1 não considera as avaliações um ativo importante, e uma avaliação ofensiva pode ser removida. Controles como moderação prévia de todas as avaliações custariam mais do que o dano que evitam.
* **Quem aprova:** a diretoria, com parecer do responsável por segurança da informação.
* **Condições:** cada avaliação fica registrada com a conta, a data e o IP de quem a enviou. Só clientes com pedido entregue podem avaliar. Avaliações denunciadas passam por moderação manual.
* **Revisão:** a cada seis meses, junto com a revisão do registro de riscos. A revisão é antecipada se as avaliações passarem a influenciar o ranking, os preços ou as vendas, ou se houver um incidente envolvendo uma avaliação.

### 4.2 Funções do NIST CSF 2.0

O NIST Cybersecurity Framework 2.0 organiza os resultados de segurança em seis funções. Cada função se divide em categorias, que descrevem um resultado esperado. Os controles são as medidas concretas que este sistema adota para alcançar esses resultados. A função não é um controle.

|Função|Finalidade|Como se aplica à plataforma|
|-|-|-|
|Govern (GV)|Definir políticas, responsabilidades, prioridades e critérios de decisão|Política de gestão de riscos e critérios de aceitação (como o do [R13]); quem cria e aprova contas internas; obrigações da LGPD; contratos com o gateway e a transportadora|
|Identify (ID)|Conhecer ativos, dependências, vulnerabilidades e riscos|Inventário dos ativos ([seção 3.2 da Etapa 1][e1-32]) e das bibliotecas do fatiador e do visualizador; este registro de riscos e a revisão dele|
|Protect (PR)|Implementar salvaguardas para reduzir a probabilidade ou o impacto|Autenticação, autorização por objeto, isolamento dos workers, criptografia e recálculo de preços no servidor|
|Detect (DE)|Identificar eventos suspeitos, falhas e possíveis incidentes|Logs de auditoria e alertas: enumeração de IDs, pico na fila de processamento, acesso interno fora do padrão e divergência na conciliação de pagamentos|
|Respond (RS)|Conter, analisar, comunicar e tratar incidentes|Revogar sessões, bloquear contas, investigar pelo log e comunicar clientes e ANPD em caso de vazamento|
|Recover (RC)|Restaurar serviços e dados e reduzir os prejuízos causados|Restaurar banco e armazenamento a partir do backup, reprocessar pedidos afetados e avisar os clientes da volta do serviço|

**Função, resultado esperado e controle**

|Função|Resultado esperado (categoria do CSF 2.0)|Controle neste sistema|
|-|-|-|
|Protect|PR.AA: o acesso aos ativos fica limitado a identidades autenticadas e autorizadas|SSO com MFA para entrar no painel interno|
|Detect|DE.CM: os ativos são monitorados para encontrar eventos adversos|Alerta quando uma conta recebe muitas respostas 403 em sequência, o que indica enumeração de IDs|
|Govern|GV.RR: papéis, responsabilidades e autoridades estão definidos|Processo documentado de criação, revisão e remoção das contas internas|

**Categorias usadas neste documento**

|Categoria|Resultado esperado|
|-|-|
|GV.RM|Prioridades, tolerância a riscos e critérios de decisão estão definidos e são usados|
|GV.RR|Papéis, responsabilidades e autoridades estão definidos e são cobrados|
|GV.SC|Os riscos de fornecedores e parceiros (gateway, transportadora e bibliotecas de terceiros) são conhecidos e tratados em contrato|
|ID.AM|Dados, sistemas, software e serviços estão inventariados e são geridos conforme sua importância|
|ID.RA|Vulnerabilidades e riscos são identificados, avaliados e priorizados|
|PR.AA|O acesso fica limitado a identidades autenticadas e autorizadas|
|PR.AT|As pessoas são treinadas para realizar suas tarefas com segurança|
|PR.DS|A confidencialidade, a integridade e a disponibilidade dos dados são protegidas|
|PR.PS|Hardware, software e serviços são configurados e mantidos de forma segura|
|PR.IR|A infraestrutura tem capacidade suficiente e isolamento entre os ambientes|
|DE.CM|Os ativos são monitorados para encontrar eventos adversos|
|DE.AE|Os eventos suspeitos são analisados para entender o ataque|
|RS.MA|O plano de resposta é executado quando um incidente é declarado|
|RS.AN|A investigação descobre a causa e o alcance do incidente|
|RS.CO|O incidente é comunicado a clientes, parceiros e autoridades|
|RC.RP|Sistemas e dados são restaurados depois do incidente|

### 4.3 Catálogo de controles

Os controles abaixo detalham as medidas da [seção 3.3](#33-recomendações-próximas). Cada um diz onde é aplicado, como funciona, quem responde por ele e como confirmar que existe e funciona. Os responsáveis são equipes ou papéis da empresa que opera a plataforma: Back-end, Front-end, Infraestrutura/DevOps, Segurança da informação, Líder de produção, Coordenação de suporte, Financeiro, Encarregado de dados (DPO) e Diretoria.

|ID|Controle (onde e como funciona)|Função e categoria NIST|Responsável|Evidência e verificação|
|-|-|-|-|-|
|<a id="c01"></a>C01|Toda rota da API passa por um único ponto de autorização, que nega por padrão e confere se o pedido, o arquivo ou o laudo pertence ao usuário. Técnicos só acessam os pedidos atribuídos a eles pelo líder de produção|Protect (PR.AA)|Back-end|Teste automatizado em cada rota: o usuário A pede um recurso do usuário B e recebe 403. A revisão de código confirma que nenhuma rota fica fora desse ponto, e cada 403 é registrado no log|
|<a id="c02"></a>C02|Pedidos, arquivos e laudos são identificados por UUID v4. O número exibido ao cliente não serve para buscar o recurso|Protect (PR.DS)|Back-end|Inspeção do esquema do banco. Teste que tenta IDs sequenciais e recebe 404|
|<a id="c03"></a>C03|O armazenamento não tem nenhum objeto público. A API gera uma URL pré-assinada válida por 5 minutos, e só depois da conferência do C01|Protect (PR.DS, PR.AA)|Infraestrutura/DevOps e Back-end|Acesso direto ao armazenamento sem assinatura retorna 403, e uma URL expirada é recusada. A configuração do armazenamento é verificada todo mês|
|<a id="c04"></a>C04|Os quatro perfis ([seção 3.1 da Etapa 1][e1-31]) são conferidos no servidor em todas as rotas de aprovação, edição de laudo e administração, sem depender do que a interface mostra|Protect (PR.AA)|Back-end|Um cliente que chama a rota de aprovação recebe 403 e o evento é registrado. A matriz de perfis por rota é revisada a cada versão|
|<a id="c05"></a>C05|O painel interno fica em um endereço separado, acessível só pela VPN ou por uma lista de IPs. O login é por SSO com MFA para todos os perfis internos|Protect (PR.AA)|Infraestrutura/DevOps e Segurança da informação|Acessos de fora da VPN e logins sem o segundo fator são recusados. O relatório do SSO mostra MFA ativo em todas as contas internas|
|<a id="c06"></a>C06|Uma conta interna só é criada a pedido do gestor e com o perfil aprovado pela segurança. Os acessos são revisados a cada três meses e removidos no mesmo dia do desligamento. Contas compartilhadas são proibidas|Govern (GV.RR), Protect (PR.AA)|Diretoria (aprova) e Segurança da informação (executa)|Ata de cada revisão trimestral. A lista de desligados do RH, comparada com as contas ativas, não mostra divergência|
|<a id="c07"></a>C07|O login limita as tentativas por conta e por IP, com espera crescente e CAPTCHA depois de 5 falhas, sem bloqueio permanente. O cliente recebe aviso de login em dispositivo novo e pode ativar o MFA|Protect (PR.AA), Detect (DE.CM)|Back-end|Teste com 10 senhas erradas: a espera aumenta e o CAPTCHA aparece, mas o dono continua entrando com a senha certa. O aviso de dispositivo novo chega por e-mail|
|<a id="c08"></a>C08|As sessões expiram depois de 30 minutos sem uso no painel e de 7 dias no site do cliente. O token é trocado a cada login e revogado no logout, e os cookies usam `HttpOnly`, `Secure` e `SameSite`|Protect (PR.AA)|Back-end|Inspeção dos cabeçalhos `Set-Cookie`. Um token reaproveitado depois do logout é recusado|
|<a id="c09"></a>C09|Tudo o que o painel exibe vindo do cliente (mensagens, avaliações e nomes de arquivo) passa por codificação de saída. As páginas do painel usam CSP sem `unsafe-inline`|Protect (PR.PS)|Front-end|Um script inserido em cada um desses campos aparece como texto. O cabeçalho CSP está presente em todas as páginas do painel|
|<a id="c10"></a>C10|O processamento usa uma fila com cota por conta (até 3 tarefas simultâneas). A API confere o tamanho, o formato e o número de polígonos antes de gravar o arquivo e de criar a tarefa. O escalonamento tem um teto de custo|Protect (PR.IR)|Back-end e Infraestrutura/DevOps|Um upload acima do limite é recusado antes da gravação. Num teste de carga com várias contas, a espera aumenta, mas o orçamento continua disponível|
|<a id="c11"></a>C11|Os workers rodam em contêineres descartáveis, sem rede, sem root e com tempo e memória máximos por tarefa. A credencial de cada worker só lê o arquivo da tarefa. Os PDFs do RF22 são convertidos em imagem dentro do worker|Protect (PR.PS, PR.IR)|Infraestrutura/DevOps|Uma tarefa de teste que tenta acessar a rede ou outro arquivo falha, e uma tarefa que passa do tempo é encerrada. A configuração do contêiner é revisada a cada versão|
|<a id="c12"></a>C12|As bibliotecas do fatiador, do visualizador e do conversor de PDF ficam inventariadas. A análise de dependências (SCA) roda a cada build, e falhas críticas são corrigidas em até 7 dias|Identify (ID.AM, ID.RA), Govern (GV.SC)|Segurança da informação e Infraestrutura/DevOps|O relatório da SCA acompanha cada versão, sem falha crítica aberta além do prazo|
|<a id="c13"></a>C13|O checkout recebe só identificadores (material, quantidade, opções e CEP). Preço, desconto progressivo e frete são recalculados no servidor, e a divergência com o valor enviado é registrada|Protect (PR.DS), Detect (DE.CM)|Back-end|Num teste com preço, desconto ou quantidade adulterados, o valor cobrado é o recalculado e o evento vai para o log|
|<a id="c14"></a>C14|O pedido só é marcado como pago depois de consultar o gateway ou validar a assinatura da notificação. O ID da transação fica vinculado ao pedido, e os pedidos pagos são conciliados todo dia com o extrato do gateway|Protect (PR.DS), Detect (DE.CM)|Back-end e Financeiro|Uma notificação sem assinatura ou com assinatura errada é recusada. O relatório diário de conciliação não tem divergência pendente|
|<a id="c15"></a>C15|O retorno de rastreio é autenticado por assinatura HMAC, com uma chave para cada transportadora definida em contrato. O status recebido é confirmado na API da transportadora antes de o cliente ser avisado|Protect (PR.AA), Govern (GV.SC)|Back-end|Uma requisição sem assinatura ou com assinatura inválida é recusada e registrada|
|<a id="c16"></a>C16|Todos os fluxos ([F1] a [F7]) usam TLS 1.2 ou superior, e o banco e o armazenamento são criptografados em repouso. O hash SHA-256 do STL é calculado no upload e conferido antes do fatiamento e da impressão. Os laudos têm histórico de versões e guardam o hash da versão enviada ao cliente|Protect (PR.DS), Detect (DE.CM)|Back-end e Infraestrutura/DevOps|A varredura de TLS não encontra protocolo fraco. Um arquivo alterado no armazenamento bloqueia o fatiamento. O hash do laudo enviado confere com a versão guardada|
|<a id="c17"></a>C17|O pagamento usa a página ou a tokenização de um gateway certificado PCI DSS, e o contrato define a guarda dos comprovantes e o processo de contestação. CPF, endereço, e-mail e dados de pagamento são mascarados nos logs, e só o serviço de pedidos acessa a API de checkout|Protect (PR.DS), Govern (GV.SC)|Back-end e Encarregado de dados (DPO)|A busca automática por padrões de CPF e cartão nos logs não encontra nada. A integração e o contrato com o gateway são revisados a cada ano|
|<a id="c18"></a>C18|O log de auditoria só aceita inclusões e fica fora da base e das permissões da aplicação. Ele registra quem, quando, o quê, o IP e os valores antes e depois para aprovações, cancelamentos, pagamentos, trocas de endereço, laudos e avaliações. Cancelamentos e trocas de endereço são confirmados ao cliente pelo canal cadastrado|Protect (PR.DS), Detect (DE.CM), Respond (RS.AN)|Infraestrutura/DevOps e Back-end|Cada ação listada gera um registro. Uma tentativa de apagar ou alterar o log com a credencial da aplicação falha|
|<a id="c19"></a>C19|Regras de alerta sobre o log e o monitoramento disparam em casos como enumeração de IDs, pico de uploads ou na fila, acesso interno fora do horário ou da rede e divergência na conciliação. O plano de resposta prevê revogar sessões, bloquear contas, investigar pelo log e comunicar clientes e ANPD|Detect (DE.AE), Respond (RS.MA, RS.AN, RS.CO)|Segurança da informação|Uma simulação de incidente por semestre (por exemplo, conta de técnico invadida), com relatório. Cada regra tem um responsável e um prazo de atendimento|
|<a id="c20"></a>C20|O banco, o armazenamento e uma cópia do log de auditoria têm backup diário em outra conta. A recuperação inclui restaurar os dados, reprocessar os pedidos afetados, recriar os workers a partir de imagens limpas e trocar as credenciais comprometidas|Recover (RC.RP)|Infraestrutura/DevOps|Um teste de restauração por semestre, com o tempo medido, e o roteiro de recuperação executado na simulação do C19|
|<a id="c21"></a>C21|O suporte só troca o endereço depois de confirmação pelo e-mail ou telefone cadastrado, e nenhuma troca é feita depois do despacho. Os atendentes são treinados contra engenharia social|Protect (PR.AA, PR.AT)|Coordenação de suporte|Auditoria mensal de uma amostra dos chamados de troca de endereço e um teste de engenharia social simulado por ano|
|<a id="c22"></a>C22|A política de gestão de riscos adota os critérios desta etapa, define que a diretoria aprova toda aceitação de risco e prevê a revisão do registro a cada seis meses, além de uma revisão extra depois de cada incidente ou mudança grande no sistema|Govern (GV.RM), Identify (ID.RA)|Diretoria e Segurança da informação|Política aprovada e publicada, e a ata de cada revisão semestral com o registro atualizado|

### 4.4 Mapeamento dos riscos para as funções do NIST

Cada X corresponde a um controle atribuído ao risco no plano da seção 4.5. A função só foi marcada quando esse controle tem papel real no tratamento daquele risco.

|Risco|Govern|Identify|Protect|Detect|Respond|Recover|
|-|:-:|:-:|:-:|:-:|:-:|:-:|
|[R01]||| X | X | X ||
|[R02]||| X | X |||
|[R03]| X || X | X | X | X |
|[R04]| X || X ||||
|[R05]||| X | X |||
|[R06]||| X | X | X | X |
|[R07]||| X | X |||
|[R08]||| X | X |||
|[R09]||| X | X |||
|[R10]| X ||| X |||
|[R11]||| X || X ||
|[R12]| X || X || X ||
|[R13]| X | X |||||
|[R14]||| X | X | X ||
|[R15]| X || X ||||
|[R16]||| X | X | X ||
|[R17]||| X | X | X ||
|[R18]||| X | X |||
|[R19]||| X | X |||
|[R20]||| X | X | X ||
|[R21]| X | X | X | X | X | X |
|[R22]||| X | X | X ||
|[R23]||| X | X |||
|[R24]||| X | X | X ||
|[R25]||| X ||| X |
|[R26]||| X ||||

**Por que algumas funções ficaram em branco:**

* **Recover** só foi marcado quando há algo a restaurar: pedidos e laudos alterados ([R03], [R06]), servidores comprometidos ([R21]) ou logs apagados ([R25]). Nos vazamentos ([R14], [R16], [R17]) não há o que recuperar, porque um dado vazado não volta a ser sigiloso. Ali o tratamento é conter e comunicar (Respond).
* **Identify** foi marcado no [R21], cuja falha está em bibliotecas de terceiros que precisam de inventário, e no [R13], que só pode ser aceito com reavaliação periódica. O inventário de ativos da Etapa 1 e este registro sustentam todos os riscos e por isso não foram marcados linha a linha.
* **Govern** aparece quando o risco depende de um processo interno ([R03], [R12]), de um contrato com terceiros ([R04], [R10], [R15], [R21]) ou de uma decisão de aceitação ([R13]).
* **Respond** ficou de fora dos riscos de dano limitado e reversível, como [R07], [R08] e [R18], em que a própria proteção resolve o caso sem abrir um incidente.
* O [R21] é o único com as seis funções. A execução de código nos workers envolve fornecedores (Govern), inventário (Identify), isolamento (Protect), alerta (Detect), contenção (Respond) e reconstrução dos servidores (Recover).
* O [R26] só tem Protect porque a estratégia é evitar: o técnico deixa de abrir o PDF original.

### 4.5 Plano de tratamento

|Risco|Estratégia|Controles propostos|Funções relacionadas|Responsáveis|Evidências e verificação|
|-|-|-|-|-|-|
|[R01] Conta de cliente invadida|Reduzir|[C07], [C08], [C19]|Protect, Detect, Respond|Back-end e Segurança da informação|Teste com 10 senhas erradas; token recusado depois do logout; aviso de login em dispositivo novo recebido|
|[R02] Desvio de entrega pelo suporte|Reduzir|[C21], [C18]|Protect, Detect|Coordenação de suporte e Back-end|Auditoria mensal dos chamados de troca de endereço; cada troca registrada no log com a confirmação do cliente|
|[R03] Falso técnico no painel|Reduzir|[C05], [C06], [C18], [C19], [C20]|Govern, Protect, Detect, Respond, Recover|Infraestrutura/DevOps, Segurança da informação e Diretoria|Acessos sem VPN ou sem MFA recusados; revisão trimestral sem divergência com o RH; simulação de conta de técnico invadida|
|[R04] Transportadora falsa|Reduzir|[C15]|Govern, Protect|Back-end|Retorno sem assinatura HMAC recusado; chave de cada transportadora registrada no contrato|
|[R05] STL alterado|Reduzir|[C16]|Protect, Detect|Back-end e Infraestrutura/DevOps|STL alterado no armazenamento bloqueia o fatiamento; varredura de TLS sem protocolo fraco|
|[R06] Pedidos de terceiros alterados|Reduzir|[C01], [C02], [C18], [C19], [C20]|Protect, Detect, Respond, Recover|Back-end, Segurança da informação e Infraestrutura/DevOps|Teste de IDOR em todas as rotas de pedido; IDs sequenciais retornam 404; alerta de 403 em sequência; status restaurados a partir do log na simulação|
|[R07] Preço alterado no checkout|Reduzir|[C13], [C14]|Protect, Detect|Back-end e Financeiro|Checkout com preço adulterado cobra o valor recalculado; conciliação diária sem divergência|
|[R08] Desconto indevido|Reduzir|[C13]|Protect, Detect|Back-end|Quantidade falsa não gera desconto, e a divergência é registrada|
|[R09] Laudo DFM alterado|Reduzir|[C04], [C16], [C18]|Protect, Detect|Back-end e Líder de produção|Só o técnico atribuído edita o laudo; hash do laudo enviado confere com a versão guardada; cada edição registrada|
|[R10] Disputa de pagamento|Compartilhar|[C14], [C17]|Govern, Detect|Financeiro e Back-end|Contrato com o gateway define comprovantes e contestação; todo pedido pago tem ID de transação; conciliação diária|
|[R11] Cliente nega o cancelamento|Reduzir|[C18]|Protect, Respond|Back-end e Coordenação de suporte|Cada cancelamento gera registro (usuário, data e IP) e confirmação no canal cadastrado|
|[R12] Técnico nega a aprovação|Reduzir|[C06], [C18]|Govern, Protect, Respond|Líder de produção e Segurança da informação|Nenhuma conta compartilhada na revisão trimestral; cada aprovação ligada a uma conta pessoal no log|
|[R13] Autoria de avaliação negada|Aceitar|[C18], [C22]|Govern, Identify|Diretoria e Segurança da informação|Aceitação aprovada em ata; cada avaliação registrada com conta, data e IP; revisão semestral|
|[R14] Download de STL de terceiros|Reduzir|[C01], [C02], [C03], [C16], [C19]|Protect, Detect, Respond|Back-end, Infraestrutura/DevOps e Segurança da informação|Troca de ID na URL retorna 403 ou 404; nenhum objeto público no armazenamento; URL expirada recusada|
|[R15] Dados de pagamento expostos|Evitar|[C17]|Govern, Protect|Back-end e Encarregado de dados (DPO)|Nenhum número de cartão no banco nem nos logs (busca automática); revisão anual da integração com o gateway|
|[R16] Dados de outros clientes|Reduzir|[C01], [C02], [C19]|Protect, Detect, Respond|Back-end e Encarregado de dados (DPO)|Teste de IDOR nas rotas de histórico, cadastro e laudo; plano de comunicação à ANPD revisado na simulação|
|[R17] Desenhos técnicos expostos|Reduzir|[C01], [C03], [C16], [C19]|Protect, Detect, Respond|Back-end, Infraestrutura/DevOps e Líder de produção|Técnico não atribuído recebe 403 ao abrir o PDF; PDFs criptografados em repouso|
|[R18] Bloqueio por força bruta|Reduzir|[C07]|Protect, Detect|Back-end|Na força bruta simulada, a espera cresce e o dono continua entrando|
|[R19] Envio abusivo de arquivos|Reduzir|[C10], [C19]|Protect, Detect|Back-end e Infraestrutura/DevOps|Upload acima do limite recusado antes da gravação; alerta de pico de uploads disparado no teste|
|[R20] Sobrecarga do processamento|Reduzir|[C10], [C11], [C19]|Protect, Detect, Respond|Infraestrutura/DevOps e Back-end|No teste de carga com várias contas, o orçamento continua disponível; tarefa acima do tempo é encerrada|
|[R21] Execução de código por STL|Reduzir|[C11], [C12], [C19], [C20]|Govern, Identify, Protect, Detect, Respond, Recover|Infraestrutura/DevOps e Segurança da informação|Worker sem rede e sem acesso a outros arquivos no teste; SCA sem falha crítica aberta; workers recriados a partir de imagem limpa na simulação|
|[R22] Autoaprovação e funções administrativas|Reduzir|[C04], [C18], [C19]|Protect, Detect, Respond|Back-end e Segurança da informação|Cliente que chama a rota de aprovação recebe 403 e o evento é registrado; matriz de perfis por rota revisada a cada versão|
|[R23] Pagamento falso|Reduzir|[C14]|Protect, Detect|Back-end e Financeiro|Notificação sem assinatura recusada; o retorno do navegador não muda o pedido para pago|
|[R24] XSS armazenado no painel|Reduzir|[C08], [C09], [C19]|Protect, Detect, Respond|Front-end, Back-end e Segurança da informação|Script em mensagem, avaliação e nome de arquivo aparece como texto; CSP e cookie `HttpOnly` presentes|
|[R25] Logs de auditoria alterados|Reduzir|[C18], [C20]|Protect, Recover|Infraestrutura/DevOps|A credencial da aplicação não consegue apagar nem alterar o log; cópia do log restaurada no teste semestral|
|[R26] PDF malicioso|Evitar|[C11]|Protect|Infraestrutura/DevOps e Líder de produção|O painel só mostra a imagem convertida, e nenhum técnico baixa o PDF original|

### 4.6 Ordem inicial de implementação

|Onda|Controles|Riscos tratados|Justificativa|
|-|-|-|-|
|0. Governança, antes do código|[C22], [C06]|[R03], [R12], [R13]|Não exigem código e custam pouco. Definem quem aprova a aceitação de riscos e quem libera as contas internas. Sem o [C06], o SSO do [C05] não tem regra para criar e remover contas|
|1. Decisões de arquitetura (Etapa 3)|[C01], [C02], [C03], [C04], [C05], [C10], [C11]|Os seis críticos ([R03], [R06], [R14], [R17], [R20], [R22]), além de [R16], [R19], [R21] e [R26]|Mudar depois sai caro: os IDs exigiriam migração de dados, a fila mudaria o contrato da API e a autorização teria de ser refeita rota a rota. O [C01] vem antes do [C03], porque a URL pré-assinada só é gerada depois da conferência do dono. O [C10] e o [C11] entram juntos porque atuam no mesmo componente|
|2. Primeira versão do sistema|[C18], [C09], [C13], [C14], [C07], [C08], [C16], [C17]|[R01], [R05], [R07], [R08], [R09], [R10], [R11], [R15], [R18], [R23], [R24], [R25]|O [C18] vem primeiro porque o log é base para os alertas do [C19] e para os riscos de repúdio. O [C09] vem em seguida porque o [R24] contornaria a VPN e o MFA da onda 1. O [C13] e o [C14] evitam prejuízo direto e são simples. Os demais protegem as contas de clientes e os dados|
|3. Antes de entrar em produção|[C19], [C20], [C12], [C15], [C21]|[R02], [R04], [R21], e reforço para os demais|O [C19] depende dos eventos gerados pelas ondas anteriores. O [C20] só faz sentido com dados reais. O [C12] entra junto com o pipeline da Etapa 7. O [C15] depende do contrato com cada transportadora. O [C21] é uma regra de atendimento e pode ser antecipado assim que o suporte começar a operar|

A ordem será revista na Etapa 3, quando as decisões da onda 1 forem detalhadas.

### 4.7 Estimativa do risco residual

Em geral, os controles reduzem a probabilidade. O impacto só cai quando o controle limita o alcance do dano. É o caso do isolamento dos workers no [R21], da fila no [R20], que deixa o sistema mais lento em vez de pará-lo, e do log nos riscos de repúdio ([R11], [R12]), em que a prova encerra a disputa. Nos vazamentos de projetos, o impacto continua 4, porque um projeto vazado continua irreversível, por isso esses riscos não ficam abaixo de Médio.

|Risco|Nível inicial|P*I residual estimado|Nível residual esperado|Condição para aceitar o residual|
|-|-|-|-|-|
|[R01]|Alto|2 * 3 = 6|Médio|Testes do C07 e do C08 aprovados e aviso de dispositivo novo funcionando. Como o MFA é opcional para clientes, a adesão é acompanhada todo mês|
|[R02]|Alto|1 * 3 = 3|Baixo|Auditoria mensal sem nenhuma troca de endereço feita sem confirmação|
|[R03]|Crítico|1 * 4 = 4|Médio|MFA em todas as contas internas, revisão trimestral sem divergência e simulação de conta de técnico invadida contida|
|[R04]|Médio|1 * 2 = 2|Baixo|Retorno sem assinatura recusado no teste|
|[R05]|Médio|1 * 3 = 3|Baixo|Arquivo alterado no armazenamento bloqueia o fatiamento no teste|
|[R06]|Crítico|1 * 4 = 4|Médio|Testes de IDOR aprovados em todas as rotas de pedido e nenhum achado de IDOR na verificação da Etapa 5|
|[R07]|Alto|1 * 3 = 3|Baixo|Teste de preço adulterado aprovado e conciliação sem divergência por 30 dias|
|[R08]|Médio|1 * 2 = 2|Baixo|Teste de quantidade falsa aprovado|
|[R09]|Médio|1 * 3 = 3|Baixo|Só o técnico atribuído edita o laudo, e o hash confere no teste|
|[R10]|Médio|1 * 2 = 2|Baixo|Contrato com o gateway assinado e todos os pedidos pagos com ID de transação|
|[R11]|Médio|3 * 1 = 3|Baixo|O cliente ainda pode negar, mas todo cancelamento tem registro e confirmação, o que encerra a disputa|
|[R12]|Médio|2 * 1 = 2|Baixo|Nenhuma conta compartilhada e todas as aprovações nominais no log|
|[R13]|Baixo|1 * 1 = 1|Baixo|Aceitação aprovada pela diretoria e revisada a cada seis meses|
|[R14]|Crítico|1 * 4 = 4|Médio|Testes de IDOR e de URL expirada aprovados, nenhum objeto público na verificação mensal e nenhum achado na Etapa 5|
|[R15]|Médio|1 * 2 = 2|Baixo|Nenhum CPF ou número de cartão encontrado na busca nos logs. Como o cartão não passa pelo sistema, o que pode vazar é só o token|
|[R16]|Alto|1 * 3 = 3|Baixo|Testes de IDOR nas rotas de histórico, cadastro e laudo aprovados|
|[R17]|Crítico|1 * 4 = 4|Médio|Técnico não atribuído recebe 403 e os PDFs estão criptografados em repouso|
|[R18]|Médio|3 * 1 = 3|Baixo|As tentativas continuam, mas no teste de força bruta o dono continua entrando|
|[R19]|Alto|2 * 2 = 4|Médio|Upload acima do limite recusado e alerta de pico testado. Um atacante com muitas contas ainda consegue causar lentidão|
|[R20]|Crítico|2 * 2 = 4|Médio|No teste de carga, o orçamento continua disponível e a espera fica dentro do limite definido pela diretoria|
|[R21]|Alto|2 * 2 = 4|Médio|Teste de isolamento aprovado e SCA sem falha crítica aberta por mais de 7 dias. Falhas ainda desconhecidas continuam possíveis|
|[R22]|Crítico|1 * 4 = 4|Médio|Testes de perfil por rota aprovados e nenhum achado de escalonamento de privilégio na Etapa 5|
|[R23]|Médio|1 * 3 = 3|Baixo|Notificação sem assinatura recusada no teste|
|[R24]|Alto|1 * 4 = 4|Médio|Teste de XSS nos campos do painel aprovado e CSP ativa. Um script que escape continuaria agindo com a sessão de um usuário interno|
|[R25]|Médio|1 * 2 = 2|Baixo|A credencial da aplicação não altera o log, e a restauração da cópia foi testada|
|[R26]|Médio|1 * 3 = 3|Baixo|O painel não oferece nenhum caminho para baixar o PDF original|

Pela estimativa, nenhum risco continuaria Alto ou Crítico: 10 ficariam Médios e 16 Baixos. Pelo C22, manter um residual Médio exige aprovação da diretoria. Esses valores são uma estimativa: a redução só será confirmada depois da implementação, dos testes e da coleta das evidências da seção 4.5.

## Parte 5 - Considerações finais

* **Riscos mais importantes:** os seis críticos ([R03], [R06], [R14], [R17], [R20] e [R22]), mais o [R21] e o [R24], que têm o mesmo alcance (todos os clientes) mas exigem mais capacidade técnica.
* **Razões da priorização:** além da pontuação, pesaram o fato de que um projeto vazado não pode ser recuperado, a quantidade de clientes afetados e o custo de corrigir depois uma decisão de arquitetura. Também subiram na lista os riscos que contornam outros controles ([R24]) e os que servem de base para outros ([R25]).
* **Estratégias predominantes:** reduzir, em 22 dos 26 riscos. Dois riscos são evitados ([R15] e [R26]), um é compartilhado com o gateway ([R10]) e um é aceito ([R13]).
* **Funções do NIST mais relevantes:** Protect, presente em 24 riscos, e Detect, em 19. Respond aparece em 12 riscos, Govern em 7, Recover em 4 e Identify em 2. Recover é pouco usado porque os riscos mais graves são vazamentos, que não têm como ser desfeitos.
* **Controles essenciais:** a autorização centralizada (C01), o acesso ao painel por SSO, MFA e VPN (C05), a fila e os workers isolados (C10 e C11) e o log de auditoria (C18). Também o C19 (alertas e resposta), que aparece em 11 riscos.
* **Principais dificuldades:**
  * distinguir evitar de reduzir no [R15] e no [R26];
  * decidir quando marcar Recover em casos de vazamento;
  * definir responsáveis para uma empresa hipotética;
  * escrever controles específicos sem multiplicar o número deles.
* **Limitações:**
  * o sistema ainda está em projeto, e as notas partem de condições supostas, não de testes;
  * os responsáveis, os prazos e os limites (5 minutos, 3 tarefas, 7 dias) são propostas;
  * a escala de 1 a 4 é grosseira;
  * o risco residual é só uma estimativa.
* **Próximas etapas:**
  * **Etapa 3:** os controles da onda 1 viram requisitos e decisões de arquitetura para os riscos críticos.
  * **Etapa 4:** detalha como práticas de código seguro o C01 (autorização por objeto) e o C13 (recálculo no servidor).
  * **Etapa 5:** verifica na prática IDOR, XSS, cabeçalhos e cookies ([R14], [R22], [R24]).
  * **Etapa 6:** usa as regras do C19 como base.
  * **Etapa 7:** traz o C12 e os testes da seção 4.5 como condições do pipeline.

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

[R01]: #22-registro-de-riscos
[R02]: #22-registro-de-riscos
[R03]: #22-registro-de-riscos
[R04]: #22-registro-de-riscos
[R05]: #22-registro-de-riscos
[R06]: #22-registro-de-riscos
[R07]: #22-registro-de-riscos
[R08]: #22-registro-de-riscos
[R09]: #22-registro-de-riscos
[R10]: #22-registro-de-riscos
[R11]: #22-registro-de-riscos
[R12]: #22-registro-de-riscos
[R13]: #22-registro-de-riscos
[R14]: #22-registro-de-riscos
[R15]: #22-registro-de-riscos
[R16]: #22-registro-de-riscos
[R17]: #22-registro-de-riscos
[R18]: #22-registro-de-riscos
[R19]: #22-registro-de-riscos
[R20]: #22-registro-de-riscos
[R21]: #22-registro-de-riscos
[R22]: #22-registro-de-riscos
[R23]: #22-registro-de-riscos
[R24]: #22-registro-de-riscos
[R25]: #22-registro-de-riscos
[R26]: #22-registro-de-riscos

[C01]: #c01
[C02]: #c02
[C03]: #c03
[C04]: #c04
[C05]: #c05
[C06]: #c06
[C07]: #c07
[C08]: #c08
[C09]: #c09
[C10]: #c10
[C11]: #c11
[C12]: #c12
[C13]: #c13
[C14]: #c14
[C15]: #c15
[C16]: #c16
[C17]: #c17
[C18]: #c18
[C19]: #c19
[C20]: #c20
[C21]: #c21
[C22]: #c22
