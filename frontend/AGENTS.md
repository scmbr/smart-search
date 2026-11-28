# Frontend: React/TypeScript и FSD

`frontend/` — React/TypeScript/Vite клиент реального lexical POST /search.
Контракт и общий Git workflow — в корневом AGENTS.md. Локальный Node 22.11
поддерживается; .nvmrc задаёт major 22 для CI. Использовать только npm/lockfile.

## Команды (из frontend/)

| Действие | Команда |
| --- | --- |
| Установка | `npm ci` |
| Запуск | `npm run dev` — 127.0.0.1:5173; API отдельно на 8080 |
| Сборка / локальный preview | `npm run build` / `npm run preview` |
| ESLint | `npm run lint` |
| Архитектура | `npm run lint:arch` — собственный TypeScript AST checker |
| Типы | `npm run typecheck` |
| Генерация API | `npm run generate:api` из ../docs/openapi.yaml |
| Проверка генерации | `npm run check:api` |
| Тесты | `npm run test:ci` — generation check, node architecture tests, Vitest |
| Зависимости | `npm audit`; версии exact, без npm audit fix --force |

scripts/check-architecture.mjs проверяет направления слоёв/public API, включая
re-export и dynamic/type imports, и размещение сетевых вызовов. Steiger и
eslint-plugin-boundaries не установлены; не выдавать собственный checker за них.
Generated types находятся в src/shared/api/generated/schema.ts: менять генерацией,
не вручную. CI запускает все пять обязательных scripts, frontend-job уже применим.
В package.json точечный override minimatch 11.0.1 для совместимого TS ESLint;
при обновлении линтера проверить необходимость override, engines и audit.

Backend запускается из корня `go run ./cmd/api`; dev/preview proxy /api →
127.0.0.1:8080. Ключи/БД/ES не доступны браузеру. HTTP hybrid/live, подсказки,
персонализация и рекомендации ещё не подключены. Браузерный e2e/визуальная QA
не заменяются Vitest DOM-тестами; явно указывать непроведённые проверки.

## Feature-Sliced Design

Структура: `frontend/src/`.
Строгая иерархия: **app → pages → widgets → features → entities → shared**.

- Импорты идут только в нижележащие слои. Запрещены импорты вверх и между
  разными слайсами одного слоя, включая обход через barrel/re-export.
- Внутри собственного слайса разрешены относительные импорты его файлов.
  Если двум слайсам нужна общая функция, вынеси её на подходящий нижний слой;
  координацию размещай в вышележащем слое.
- Любой внешний импорт из слайса — только через его корневой `index.ts`.
  Deep imports во внутренние `ui/model/api/lib/config` чужого слайса запрещены.
- Сегменты слайса: `ui/`, `model/`, `api/`, `lib/`, `config/`; создавай только нужные.
- `app`: запуск, провайдеры, роутинг, подключение зависимостей; `pages`: экраны;
  `widgets`: составные блоки; `features`: пользовательские действия;
  `entities`: представления бизнес-сущностей; `shared`: общие технические средства.
- `app` и `shared` не содержат продуктовых слайсов. Для общих модулей/сегментов
  shared сохраняй явные public API (`shared/ui/index.ts`, `shared/api/index.ts` и т.п.).

## UI, состояние и сеть

- Не вызывай fetch/axios из UI-компонентов и не обращайся из браузера к БД,
  Elasticsearch или OpenAI. Сеть — в `api/` сегменте либо `shared/api`.
- UI отображает состояние и вызывает действия; сценарии и переходы состояния
  тестируются в model/lib, а маппинг транспортных данных остаётся в api.
- Сначала ищи существующие компоненты в shared/ui, хуки и функции нижних слоёв;
  не копируй их между слайсами. Текущие общие API и UI находятся в shared/api и shared/ui.
- После настройки кодогенерации переиспользуй API-типы через public API shared/api;
  не дублируй DTO вручную и не редактируй сгенерированные файлы.
- Для поиска учитывай loading/error/empty/degraded, отмену или игнорирование
  устаревших ответов и клавиатурное управление подсказками.
- Ранжирование, цены, лицензии и проверка допродаж принадлежат backend: клиент
  показывает полученные решения, а не пересчитывает их по собственной политике.
- Не рендери необработанный HTML каталога. Тестируй сценарии и состояния интерфейса,
  а не только факт рендера компонентов.
