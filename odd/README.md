# ODD workflow

Use ODD task files to make substantial work and its evidence resumable. Keep the task file as the durable project record and mirror its task context in Engram under `odd/<feature>/tasks`.

## Task files

- Name feature task files `odd/tasks/<feature>.md`, using a concise kebab-case feature name.
- Create the task file before the first implementation write. Define objective, accepted scope, tasks, checks, and acceptance criteria before coding.
- Keep tasks small and independently verifiable. Each task records its route, scope, files, checks, evidence, and commit identity.
- Mark a checkbox complete only after observing its listed checks. Record the exact checks and outcomes, relevant evidence, and commit ID next to the task. Never invent a commit ID; use `pending` until one exists.
- Update the Engram mirror topic `odd/<feature>/tasks` as the task plan or verified progress changes. Keep it aligned with the task file.

## Resume work

1. Call `mem_context` to recover relevant project context.
2. Use `mem_search` for the feature's `odd/<feature>/tasks` topic.
3. Read `odd/tasks/<feature>.md` and reconcile the durable checklist with the Engram observations before continuing.

## Naming and closing

Use the same feature slug in the filename and Engram topic. Link applicable ADRs from the task file. At closure, ensure every completed task has observed verification evidence and its commit ID, record remaining risks or follow-ups, update the Engram mirror, and state the next step. Do not close unfinished work as complete.
