# TODO / SDD changes

Планы крупных фич + точки отката. Полные правила агента: **[`../AGENTS.md`](../AGENTS.md)**.

## Структура

```
TODO/
  README.md
  active/
    YYYY-MM-DDTHHMMSS-<slug>/        ← одна активная крупная фича
      plan.md
      tasks.md
  archive/
    YYYY-MM-DD-<slug>/               ← иммутабельно
      plan.md
      tasks.md
      ROLLBACK.md                    ← git tag + как откатиться
```

## Старт новой крупной фичи

1. **Архивировать** текущую `active/` (если есть):
   - закрыть `tasks.md`;
   - закоммитить код фичи (если пользователь просит коммиты);
   - создать tag `archive/YYYY-MM-DD-<slug>`;
   - написать `ROLLBACK.md`;
   - перенести папку в `archive/YYYY-MM-DD-<slug>/`.
2. Создать новую `active/YYYY-MM-DDTHHMMSS-<slug>/` с `plan.md` + `tasks.md`.

## Во время фичи

- Обновлять только `tasks.md` (чекбоксы) и living docs (`docs/*`, swagger).
- Не править содержимое уже лежащего в `archive/`.

## Откат

См. `archive/<feature>/ROLLBACK.md` и `git tag -l 'archive/*'`.
