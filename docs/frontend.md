# Минимальный клиент поиска

React/TypeScript/Vite, npm + package-lock, FSD. Браузер обращается только к
same-origin `/api/v1/search`; dev/preview proxy направляет `/api` на Go
`127.0.0.1:8080`. Нет ключей OpenAI, прямых БД/ES-вызовов или mock-каталога.
Текущий backend HTTP — lexical; интерфейс не обещает подключённый live hybrid.

## Запуск

В первом терминале из корня: `go run ./cmd/api -name-catalog artifacts/name-catalog.json`
(migration 005 и подготовленный словарь уже нужны; [подготовка](named-search.md)).
Без словаря можно запустить `go run ./cmd/api`, но новый поиск названий будет выключен.
Во втором из `frontend/`:

```powershell
npm ci
npm run dev
```

Открыть http://127.0.0.1:5173. Для сборки: `npm run build`; локальная проверка
сборки с тем же API proxy: `npm run preview` (порт 4173). Go пока не раздаёт dist,
публичное размещение и production reverse proxy в этот шаг не входят.

## Поведение и приёмка

- Название/версия/артикул, фильтры availability и offer_kind; Enter и кнопка
  отправляют поиск, примеры выполняют реальные запросы. Подсказки ещё не реализованы.
- Каждая новая отправка получает новый Idempotency-Key, retry ошибки использует
  прежний ключ/тело. Session ID хранится в sessionStorage, при запрете storage —
  только на время работы страницы. Автоматических повторов API-запросов нет.
- Изменение запроса/фильтров и размонтирование отменяют текущий запрос.
  Счётчик запросов игнорирует устаревший ответ даже при игнорировании AbortSignal.
- Loading/error/empty/degraded/shortfall отдельны. Карточки сохраняют порядок API.
  Клиент не ранжирует и не подбирает допродажи; top4/shortfall берутся из backend.
- Нельзя объявлять число карточек точным global total: null не превращается в 0.
  Неизвестная цена — «Цена по запросу», недоступность без обещания «под заказ».
- Family confirmed означает идентичность, не проверенную лицензию. Недобор
  четырёх семейств показан явно; recommendations и verifier ещё не подключены.
- Каталожный HTML не исполняется: React text, без dangerouslySetInnerHTML.
  Товарные ссылки HTTPS store.softline.ru, новая вкладка с noopener/noreferrer.
- Подписи полей, focus-visible, live status, мобильный layout и reduced motion.
  Проверить desktop/mobile, реальную выдачу, отсутствие API и работу клавиатуры.

## Контракт и проверки

`npm run generate:api` создаёт типы из `docs/openapi.yaml`; generated-файл
не редактируется вручную. `npm run check:api` проверяет отсутствие расхождений.
`npm run lint`, `npm run lint:arch`, `npm run typecheck`, `npm run test:ci`,
`npm run build` обязательны локально и в уже существующем CI frontend-job.

Архитектурный checker использует TypeScript AST: направление слоёв, запрет
cross-slice/deep imports (включая re-export/dynamic imports) и сети вне api.
Это собственная проверка проекта, не установленный Steiger/plugin-boundaries.
Её отрицательные fixtures входят в test:ci. Model тестирует stale response и
retry key, клиент — запрос/безопасные ошибки, карточка — цену/наличие/HTML.

## Факт проверки и оставшиеся ограничения

Lint/FSD/typecheck, generation check, node architecture tests и Vitest DOM/model
проверки выполнены. Vite proxy → настоящий API проверен AnyDesk SKU 402265,
mode=lexical, total_matches=null; страница отдаётся HTTP 200.
Встроенный браузер в чате недоступен; инструмент не получил разрешение на Chrome.
Поэтому визуальная desktop/mobile и полноценная browser e2e QA не выполнены.
Responsive CSS реализован, но это не равно визуально подтверждённой адаптивности.

Зависимости exact: Vite 6.4.6, Vitest 4.1.12 совместимы с локальным Node 22.11;
ESLint 9/TS ESLint 8.46.1 используются с точечным patched minimatch 11.0.1.
Это явный override transitive dependency; lint/архитектурные тесты проверены.
На момент настройки npm audit: 0 известных уязвимостей; CI пользуется lockfile.

HTTP hybrid/live подключён на стороне backend: [http-hybrid.md](http-hybrid.md).
Клиент отображает mode/degraded_reasons; запуск по умолчанию остаётся lexical.
