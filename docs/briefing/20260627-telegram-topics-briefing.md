# Project Briefing: NotiCLI - Telegram Topics por Recipient

**Data**: 2026-06-27
**Status**: Draft
**Versao**: 1.0

---

## 1. Visao e Proposito

**O que e**: Uma evolucao do canal Telegram do NotiCLI para permitir que cada destinatario escolha receber notificacoes em chat privado ou em um supergrupo organizado por topicos.

**Problema que resolve**: Notificacoes Telegram em chat privado ficam todas juntas e exigem prefixo visual no titulo para identificar a origem. Para uso operacional com muitos sistemas chamadores, o usuario precisa separar mensagens por origem sem editar manualmente o JSON a cada novo `--sender`.

**Proposta de valor**: Organizar automaticamente notificacoes Telegram por `--sender` usando topicos de supergrupo quando o destinatario escolher esse modo, mantendo o chat privado simples para quem preferir receber tudo junto.

## 2. Usuarios e Stakeholders

| Ator | Papel | Acoes Principais |
|------|-------|------------------|
| Aplicacao externa | Chamador da CLI | Executa `noticli send` com `--sender`, destinatario, canal, titulo e mensagem |
| Destinatario/recipient | Recebedor da notificacao | Escolhe receber Telegram em chat privado ou em supergrupo com topicos |
| Operador/admin | Configurador | Configura bot, recipients, supergrupo e permissoes iniciais |
| Bot Telegram | Integracao externa | Envia mensagens, cria topicos e recebe comandos administrativos futuros |

**Stakeholders de decisao**: Usuario do projeto.

## 3. Escopo

### MVP (Essencial)

1. Permitir configuracao Telegram por recipient para escolher entre chat privado e supergrupo com topicos.
2. Em chat privado, manter identificacao visual do sender no titulo da mensagem usando o padrao `[sender] title`.
3. Em supergrupo com topicos, usar `--sender` para identificar/criar/reutilizar o topico e enviar a mensagem sem repetir `[sender]` no titulo.
4. Criar automaticamente um topico quando nao existir mapeamento conhecido para o `--sender`.
5. Persistir o mapeamento `recipient + chat_id + sender -> message_thread_id` em arquivo separado de estado, sem misturar dados gerados em runtime no arquivo principal de configuracao.
6. Reutilizar topicos conhecidos pelo arquivo de estado local.
7. Tratar falha de envio para topico conhecido como indicio de topico invalido, removido, fechado ou desatualizado, permitindo recuperacao/criacao conforme regra futura.

### Pos-MVP (Desejavel)

1. Comandos administrativos no Telegram para associar topicos existentes a senders, por exemplo `/noticli_bind ProdSmoke`.
2. Comandos para listar e remover associacoes conhecidas, por exemplo `/noticli_topics` e `/noticli_unbind ProdSmoke`.
3. Leitura periodica ou webhook para processar comandos do bot sem edicao manual de JSON.
4. Politica de TTL/verificacao do cache de topicos.
5. Recuperacao parcial de estado a partir de updates recebidos pelo bot.

### Fora de Escopo

- Criar supergrupos automaticamente pelo bot.
- Adicionar usuarios automaticamente a grupos.
- Listar todos os topicos existentes via Telegram Bot API.
- Depender de edicao manual do JSON principal para cada novo `--sender`.
- Publicar alteracoes no GitHub sem autorizacao explicita.

## 4. Prioridades e Trade-offs

**Ordem de prioridade**: Robustez operacional > Seguranca de credenciais > UX do operador > Simplicidade inicial > Escopo completo.

**Decisoes explicitas**:
- A preferencia de entrega Telegram e por recipient, nao global do canal.
- O modo supergrupo deve criar topicos automaticamente para senders desconhecidos.
- O estado gerado em runtime deve ficar em arquivo separado, por exemplo `/opt/NotiCLI/state/telegram-topics.json`.
- A Telegram Bot API nao fornece listagem completa de topicos, entao o NotiCLI deve tratar o arquivo de estado local como fonte operacional de verdade para reuso automatico.
- Tentar deduzir topico apenas por nome nao e confiavel, pois nomes duplicados podem existir.

## 5. Restricoes

| Restricao | Valor | Notas |
|-----------|-------|-------|
| Prazo | Evolucao incremental durante configuracao de producao inicial | A producao ainda esta sendo levantada/testada |
| Equipe | Usuario + Codex | Equipe minima |
| Budget | Custo zero | Usar Telegram Bot API e arquivos locais |
| Tecnica | Go CLI, arquivo de configuracao e arquivo de estado local | Deve preservar contrato nao interativo do NotiCLI |
| Telegram API | Sem listagem completa de topicos | Recuperacao automatica total nao e possivel apenas pela API |

## 6. Stack Tecnica

| Camada | Tecnologia | Justificativa |
|--------|------------|---------------|
| CLI/Core | Go | Stack existente do NotiCLI |
| Integracao | Telegram Bot API | Canal Telegram ja implementado para `sendMessage` |
| Estado local | Arquivo JSON separado | Evita misturar configuracao declarativa com dados gerados em runtime |
| Infraestrutura | Servidor Linux em `/opt/NotiCLI` | Ambiente de producao inicial existente |
| Futuro listener | Webhook ou polling/cron | Necessario para comandos administrativos pelo Telegram |

## 7. Qualidade e Padroes

**Padroes adotados**:
- Manter execucao de envio nao interativa.
- Nao expor token Telegram, credenciais SMTP ou secrets em diagnosticos.
- Separar configuracao de recipient/canal de estado operacional gerado pelo bot.
- Testar roteamento Telegram em chat privado e supergrupo com topicos.
- Testar que o modo supergrupo nao repete `[sender]` no titulo da mensagem enviada ao topico.
- Testar recuperacao quando o topico em cache falhar.

**Compliance**: Nenhum compliance especifico definido.

## 8. Visao de Futuro

**6 meses**: NotiCLI deve permitir que usuarios organizem Telegram por topicos sem editar JSON manualmente, usando comandos do bot para bind/list/unbind e criacao automatica para novos senders.

**12 meses**: A configuracao operacional do Telegram pode evoluir para modo assistido por webhook ou polling, com menor dependencia de operacao manual no servidor.

**Riscos conhecidos**:
- Perda do arquivo de estado pode causar recriacao de topicos e duplicidade de nomes.
- Topicos criados manualmente antes do NotiCLI conhecer seus IDs exigem bind manual assistido.
- Falhas de permissao do bot em supergrupo podem impedir criacao de topicos.
- Como a Bot API nao lista topicos, nao ha garantia de recuperacao automatica completa.

---

## Itens a Definir

| Item | Dimensao | Impacto |
|------|----------|---------|
| Formato exato de `/opt/NotiCLI/state/telegram-topics.json` | Stack Tecnica | Alto |
| Politica de locking/concorrrencia para multiplas execucoes simultaneas criando topico do mesmo sender | Qualidade e Padroes | Alto |
| Politica de TTL/verificacao do cache de topicos | Qualidade e Padroes | Medio |
| Regras de recuperacao quando envio para `message_thread_id` falhar | Escopo | Alto |
| Comandos administrativos suportados pelo bot e autorizacao de quem pode executa-los | Escopo/Seguranca | Alto |
| Estrategia futura: webhook, polling por cron ou comando manual para processar updates | Stack Tecnica | Medio |

---

**Proximo passo recomendado**: `Dev Pipeline - 3. Specification - Specify` para transformar este briefing em especificacao funcional da feature Telegram Topics por Recipient.
