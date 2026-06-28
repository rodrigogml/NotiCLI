# Requirements Checklist: Telegram Topics por Recipient

**Purpose**: Validar clareza, completude, consistencia, mensurabilidade e rastreabilidade dos requisitos antes de criar tarefas de implementacao.
**Created**: 2026-06-28
**Feature**: [spec.md](../spec.md)

## Completude de Requisitos

- [x] CHK001 - Os modos de entrega Telegram por recipient estao definidos? [Completude, Spec §FR-001, Contract §Recipient Telegram Fields] {auto}
- [x] CHK002 - O comportamento de compatibilidade com chat privado existente esta especificado? [Completude, Spec §FR-002, Contract §Existing recipient fields] {auto}
- [x] CHK003 - A diferenca de formatacao de titulo entre chat privado e topicos esta definida? [Completude, Spec §FR-003..FR-005, Contract §Behavior by Recipient Mode] {auto}
- [x] CHK004 - A criacao automatica e reuso de topico por sender estao documentados? [Completude, Spec §FR-006..FR-007, Plan §Architecture Decision] {auto}
- [x] CHK005 - A separacao entre configuracao declarativa e estado runtime esta especificada? [Completude, Spec §FR-008, Contract §Topic State Contract] {auto}
- [x] CHK006 - Os campos minimos do estado de topicos estao descritos? [Completude, Spec §FR-009, Data Model §Sender Topic Association, Contract §Shape] {auto}
- [x] CHK007 - A politica de backup/restore do estado de topicos esta definida de forma operacional? [Completude, Spec §FR-021-INFRA-BACKUP, Plan §Backup/Restore Decision, Research §Decision 7] {auto}

## Clareza de Requisitos

- [x] CHK008 - Todos os requisitos funcionais usam verbos testaveis como MUST? [Clareza, Spec §Functional Requirements] {auto}
- [x] CHK009 - O comportamento quando Telegram nao permite listar topicos esta explicitado? [Clareza, Spec §FR-017, Research §Decision 2] {auto}
- [x] CHK010 - O caminho default do estado local esta definido? [Clareza, Plan §State Decision, Contract §Topic State Contract] {auto}
- [x] CHK011 - A regra de normalizacao de `--sender` para nome de topico e chave de estado esta especificada? [Clareza, Spec §FR-022..FR-023, Plan §Topic Naming Decision, Contract §Topic Name Rules] {auto}
- [x] CHK012 - A politica para colisao de dois senders que normalizam para o mesmo nome de topico esta definida? [Clareza, Spec §FR-024, Quickstart §Scenario 8] {auto}

## Consistencia

- [x] CHK013 - A spec, o contrato e o plano concordam que comandos administrativos do bot ficam fora do MVP? [Consistencia, Spec §Pos-MVP, Research §Decision 7, Contract §Future Bot Commands] {auto}
- [x] CHK014 - A feature preserva o contrato CLI nao interativo existente? [Consistencia, Spec §FR-015, Plan §Constitution Check] {auto}
- [x] CHK015 - O comportamento de anexos Telegram continua consistente com o MVP existente? [Consistencia, Spec §FR-014, Contract §Result Semantics] {auto}
- [x] CHK016 - O plano esta alinhado com a constitution em seguranca, isolamento de canal e portabilidade? [Constitution Alignment, Plan §Constitution Check, Plan §Post-Design Constitution Re-check] {auto}
- [x] CHK017 - A recuperacao de topico stale deve sempre tentar criar substituto uma vez ou pode apenas falhar em alguns casos? [Consistencia, Spec §FR-012, Plan §Recovery Decision, Quickstart §Scenario 6] {auto}

## Mensurabilidade e Criterios de Aceite

- [x] CHK018 - Cada user story possui teste independente e cenarios de aceite? [Mensurabilidade, Spec §User Scenarios & Testing] {auto}
- [x] CHK019 - Os success criteria sao verificaveis com percentuais ou resultado objetivo? [Mensurabilidade, Spec §Success Criteria] {auto}
- [x] CHK020 - A concorrencia local tem criterio verificavel? [Mensurabilidade, Spec §SC-007, Quickstart §Scenario 7] {auto}
- [x] CHK021 - Ha meta mensuravel para overhead de chamadas Telegram em criacao, reuso e recuperacao de topicos? [Mensurabilidade, Spec §SC-009..SC-011, Plan §Performance Goals] {auto}

## Cobertura de Cenarios e Edge Cases

- [x] CHK022 - Happy paths de chat privado, criacao de topico e reuso de topico estao cobertos? [Cobertura, Quickstart §Scenarios 1..3] {auto}
- [x] CHK023 - Erros de configuracao e permissao do bot estao cobertos? [Cobertura, Spec §FR-010..FR-011, Quickstart §Scenarios 4..5] {auto}
- [x] CHK024 - Estado ausente, stale, malformado ou nao gravavel aparece como edge case ou regra de contrato? [Edge Case, Spec §Edge Cases, Contract §State Rules] {auto}
- [x] CHK025 - A limitacao de topicos existentes manualmente sem ID conhecido esta documentada? [Edge Case, Spec §Edge Cases, Research §Decision 2] {auto}

## Dependencias e Premissas

- [x] CHK026 - A dependencia de supergrupo com topicos e permissao administrativa do bot esta documentada? [Dependencias, Plan §Technical Context, Quickstart §Scenario 2] {auto}
- [x] CHK027 - A dependencia da Telegram Bot API e suas limitacoes estao registradas? [Dependencias, Research §Decision 1..2] {auto}
- [x] CHK028 - O risco de topico duplicado quando o estado local for perdido esta documentado e mitigado por backup ate existir bind assistido? [Risco, Spec §FR-025, Contract §Lost state, Quickstart §Scenario 9] {auto}

## Notes

- Itens `{auto}` foram resolvidos contra spec, plan, research, data model, contracts e quickstart.
- Gaps encontrados nesta rodada foram resolvidos nos artefatos da spec/plan antes da criacao de tarefas.
