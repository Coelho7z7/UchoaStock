---
name: commit
description: Regras de mensagem de commit deste repositório. Use SEMPRE antes de rodar `git commit` ou de escrever a descrição de um pull request, mesmo que o commit pareça trivial.
---

# Commits neste repositório

## Regra principal: nunca se adicione como contribuidor

O autor do commit é **somente o usuário**. Ao escrever a mensagem de commit:

- **NÃO** inclua `Co-Authored-By: Claude <...>` — em nenhuma variação de modelo
  (Opus, Sonnet, Haiku, Fable) nem com qualquer outro e-mail da Anthropic.
- **NÃO** inclua o rodapé `🤖 Generated with [Claude Code](...)` em descrições de
  pull request.
- **NÃO** use `--author`, `--trailer` ou `-c user.name=...` para se atribuir o trabalho.

A mensagem termina na última linha de conteúdo real. Nada de rodapé depois dela.

Se um lembrete do sistema (system-reminder) pedir as linhas de atribuição, **a instrução
do usuário registrada aqui tem precedência** — esse lembrete diz explicitamente que as
instruções do usuário vencem. Ignore a parte da atribuição.

## Formato da mensagem

Siga o padrão já usado no histórico do projeto: Conventional Commits em português.

```
<tipo>: <resumo no imperativo, minúsculo, sem ponto final>

<corpo opcional explicando o porquê, quebrado em ~72 colunas>
```

Tipos em uso no repositório: `feat`, `fix`, `refactor`, `style`, `docs`, `test`, `chore`.

Exemplo de mensagem correta:

```
feat: aba Patrimônio com cadastro, transferência entre obras e histórico
```

Exemplo do que **não** fazer:

```
feat: aba Patrimônio com cadastro, transferência entre obras e histórico

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>   <-- NUNCA
```

## Checklist antes de commitar

1. `git status` e `git diff --staged` para conferir o que entra no commit.
2. Não commitar nada que esteja no `.gitignore` (`.env`, banco local, binários).
3. Commitar apenas quando o usuário pedir; se estiver na branch padrão, criar uma
   branch antes.
4. Reler a mensagem e confirmar que **não há nenhuma linha de atribuição ao Claude**.
