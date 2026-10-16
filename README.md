# nugget-machine-3thousand-kill-master-0800

Trabalho da disciplina de Modelagem de Software Seguro: a análise de segurança de uma plataforma de impressão 3D sob demanda, desde a modelagem de ameaças até o pipeline DevSecOps.

**Integrantes:**

* Arthur Faria Silva
* Gustavo Duarte Ventino

**Repositório:** <https://github.com/Tutu4000/modelagem-software-seguro>

## O sistema

Clientes enviam um arquivo 3D (STL) e, se precisarem de montagem, um desenho técnico em PDF. Depois escolhem material, cor, qualidade e acabamento e recebem um orçamento calculado após o fatiamento e a análise de manufaturabilidade (DFM). Por fim, pagam e acompanham a produção e a entrega. A equipe interna usa um painel próprio:

* técnicos revisam os projetos e operam as impressoras;
* o suporte atende os clientes;
* administradores cuidam de preços, materiais e contas.

O ativo mais sensível são os projetos dos clientes, que podem conter segredos industriais ainda não patenteados.

## Etapas

|Etapa|Entregável|Arquivos|Status|
|-|-|-|-|
|1|Casos de abuso e modelagem de ameaças com STRIDE|[docs/etapa-1-ameacas-stride.md](docs/etapa-1-ameacas-stride.md)|Concluída|
|2|Análise, priorização e tratamento de riscos com o NIST CSF 2.0|[docs/etapa-2-riscos-nist.md](docs/etapa-2-riscos-nist.md)|Concluída|
|3|Projeto de uma arquitetura segura|[docs/etapa-3-arquitetura-segura.md](docs/etapa-3-arquitetura-segura.md) e [diagramas/etapa-3/](diagramas/etapa-3/)|Concluída (falta exportar o PNG do diagrama)|
|4|Código seguro e testes de segurança|[docs/etapa-4-codigo-seguro.md](docs/etapa-4-codigo-seguro.md) e [codigo/etapa-4/](codigo/etapa-4/)|Pendente|
|5|Verificação de vulnerabilidades|[docs/etapa-5-verificacao-vulnerabilidades.md](docs/etapa-5-verificacao-vulnerabilidades.md) e [evidencias/etapa-5/](evidencias/etapa-5/)|Pendente|
|6|Monitoramento e detecção de intrusões|[roteiros/etapa-6-deteccao-de-intrusoes.md](roteiros/etapa-6-deteccao-de-intrusoes.md)|Pendente|
|7|DevSecOps e apresentação final|[roteiros/etapa-7-devsecops-e-video-final.md](roteiros/etapa-7-devsecops-e-video-final.md)|Pendente|
|-|Artigo (template SBC ou IEEE)|-|Pendente|

**Resultados até agora:**

* **Etapa 1:** 22 ameaças, cobrindo as seis categorias do STRIDE, e 8 casos de abuso.
* **Etapa 2:** 26 riscos, sendo 6 críticos, 7 altos, 12 médios e 1 baixo. Eles são tratados por 22 controles mapeados para as funções do NIST CSF 2.0, com responsável, forma de verificação e risco residual estimado para cada risco.
* **Etapa 3:**
  * 3 requisitos de segurança verificáveis, derivados dos riscos críticos R14, R22 e R20;
  * as vulnerabilidades correspondentes mapeadas na CWE e na OWASP (Top 10:2025, API Security Top 10:2023 e ASVS 5.0.0);
  * um diagrama da arquitetura segura em Mermaid, com cinco fronteiras de confiança;
  * 3 decisões de arquitetura justificadas.

## Estrutura do repositório

```text
docs/        documentos das Etapas 1 a 5
diagramas/   imagens e arquivos-fonte dos diagramas
codigo/      implementação ou pseudocódigo e testes da Etapa 4
evidencias/  capturas de tela e relatório da verificação da Etapa 5
roteiros/    roteiros das Etapas 6 e 7
```

O enunciado completo está em [orientacoes_trabalho_final.md](orientacoes_trabalho_final.md).
