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

## 2. Vulnerabilidades catalogadas

Para cada requisito, a vulnerabilidade foi buscada na CWE (MITRE) e confirmada nas referências da OWASP, nas versões vigentes em outubro de 2026: OWASP Top 10:2025, OWASP API Security Top 10:2023, ASVS 5.0.0 e OWASP Cheat Sheet Series.

|Risco|Vulnerabilidade ou categoria|Referência utilizada|Relação com o sistema|
|-|-|-|-|
|[R14]|[CWE-639]: Authorization Bypass Through User-Controlled Key, conhecida como IDOR|[A01:2025] Broken Access Control, que lista a CWE-639 e cita a alteração de parâmetros na URL; [API1:2023] Broken Object Level Authorization; [ASVS 5.0.0, requisito 8.2.2][asvs-v8]: o acesso a cada item de dado é restrito a quem tem permissão explícita sobre ele, contra IDOR e BOLA; [IDOR Prevention Cheat Sheet][cs-idor]|O visualizador 3D carrega o STL por um endereço que contém o identificador do arquivo. Se o servidor não confere o dono, basta trocar esse identificador para baixar o projeto de outro cliente ([CA01]). A cheat sheet indica a conferência do dono em cada objeto e trata os IDs aleatórios só como camada extra, que é a combinação adotada no RS01 ([C01] com [C02])|
|[R22]|[CWE-602]: Client-Side Enforcement of Server-Side Security, com [CWE-285]: Improper Authorization|[A01:2025] Broken Access Control, segundo a qual o controle de acesso só é eficaz em código confiável no servidor; [API5:2023] Broken Function Level Authorization; [ASVS 5.0.0, requisitos 8.3.1 e 8.2.1][asvs-v8]: a autorização é aplicada em uma camada de serviço confiável, e cada função só é liberada a quem tem permissão explícita; [Authorization Cheat Sheet][cs-authz]: *Deny by Default* e *Validate the Permissions on Every Request*|A interface esconde o botão de aprovar, mas a rota de aprovação responde a qualquer usuário autenticado ([CA05]). O alvo é uma **função** administrativa, e não um objeto de outro cliente, por isso a categoria é BFLA (API5), e não BOLA (API1). O RS02 tira essas rotas da API pública e faz o servidor conferir o perfil em toda requisição|
|[R20]|[CWE-770]: Allocation of Resources Without Limits or Throttling, com [CWE-400]: Uncontrolled Resource Consumption|[API4:2023] Unrestricted Resource Consumption, que cita tamanho máximo de upload, tempo de execução e memória por tarefa; [ASVS 5.0.0][asvs-v5], requisitos 5.2.1 (só aceitar arquivos que possam ser processados sem negação de serviço), 5.2.2 (conferir se o conteúdo corresponde à extensão) e 5.2.4 (cota de arquivos por usuário), e [requisito 2.4.1][asvs-v2] (controles contra automação que esgota cotas e recursos caros); [A10:2025] Mishandling of Exceptional Conditions e [CWE-636]: Not Failing Securely ('Failing Open'), para o comportamento quando os workers caem; [File Upload Cheat Sheet][cs-upload]|O fatiamento e a análise DFM consomem muita CPU e memória. Sem limites, uma única conta com modelos de milhões de triângulos ocupa os workers de todos ([CA07]). O RS03 aplica os limites antes de gravar e de enfileirar, e define que a falha dos workers deixa o orçamento "em processamento", sem liberar o pedido|

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

[CWE-285]: https://cwe.mitre.org/data/definitions/285.html
[CWE-400]: https://cwe.mitre.org/data/definitions/400.html
[CWE-602]: https://cwe.mitre.org/data/definitions/602.html
[CWE-636]: https://cwe.mitre.org/data/definitions/636.html
[CWE-639]: https://cwe.mitre.org/data/definitions/639.html
[CWE-770]: https://cwe.mitre.org/data/definitions/770.html
[A01:2025]: https://top10.owasp.org/2025/A01_2025-Broken_Access_Control/
[A10:2025]: https://top10.owasp.org/2025/A10_2025-Mishandling_of_Exceptional_Conditions/
[API1:2023]: https://api-security.owasp.org/editions/2023/en/0xa1-broken-object-level-authorization/
[API4:2023]: https://api-security.owasp.org/editions/2023/en/0xa4-unrestricted-resource-consumption/
[API5:2023]: https://api-security.owasp.org/editions/2023/en/0xa5-broken-function-level-authorization/
[asvs-v2]: https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x11-V2-Validation-and-Business-Logic.md
[asvs-v5]: https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x14-V5-File-Handling.md
[asvs-v8]: https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md
[cs-authz]: https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html
[cs-idor]: https://cheatsheetseries.owasp.org/cheatsheets/Insecure_Direct_Object_Reference_Prevention_Cheat_Sheet.html
[cs-upload]: https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html
