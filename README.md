# Умный поиск Softline

Локальное демо: поиск покупателя, рекомендации с источниками, аналитика сотрудников
и отдельное представление обработки запроса. Backend — Go, frontend — React/TypeScript,
поисковый индекс — Elasticsearch, данные и события — PostgreSQL, модели — OpenAI API.

## Текущее состояние

Доступны импорт и версионированные PostgreSQL-снимки, Elasticsearch-индекс,
CLI лексического и гибридного поиска (query-v8 / lexical-v7 / hybrid-v4),
полный кэш эмбеддингов и выборочный LLM-разбор с общим бюджетом $5.
HTTP-сервер `cmd/api` реализует `/api/v1/search` в lexical режиме с постоянным
replay и health endpoints. Frontend доступен в `frontend/`; HTTP hybrid/live, verifier и аналитика
ещё не подключены. [HTTP-поиск и ограничения](docs/http-search.md).

На development: tuning group@1/@4 52/52, task probes @4 8/16, @1 6/16.
Это не доказательство качества всех четырёх товаров. Независимое сравнение
старого hybrid-v2 не показало превосходства над baseline; новые изменения
независимо ещё не измерены. Актуальный статус и очередь — [todo.md](todo.md).

```powershell
docker compose --profile search up -d --wait postgres elasticsearch
go run ./cmd/catalog-import -migrate-only
go run ./cmd/api
```

Во втором терминале:

```powershell
cd frontend
npm ci
npm run dev
```

Открыть http://127.0.0.1:5173 — поиск, реальные карточки и фильтры.
Команды проверок и границы: [frontend](docs/frontend.md).

Нужны `.env` с PostgreSQL/ES, применённые миграции и готовые каталог/индекс
для `/health/ready`. Сервер не импортирует данные и не вызывает OpenAI.
По умолчанию: http://127.0.0.1:8080/api/v1/health/live;
[критерии готовности и запуск](docs/http-server.md).

- [Спецификации](docs/README.md)
- [Контракт OpenAPI 3.0.3](docs/openapi.yaml)
- [Решения](decisions.md)
- [План](todo.md)
- [Правила разработки](AGENTS.md)
- [Тестовые данные](testdata/README.md)

## Проверки на текущем этапе

Нужен Go 1.25 или новее. Первый запуск загружает Go-модули,
следующие работают с локальным кэшем.

```powershell
go test ./...
go vet ./...
go build ./...
gofmt -l internal cmd contracts
```

Docker, исходный feed и ключ OpenAI для этих проверок не нужны. Тесты не вызывают
OpenAI. PostgreSQL/Elasticsearch-интеграции пропускаются без TEST_POSTGRES_DSN /
TEST_ELASTICSEARCH_URL; с этими переменными используют реальные сервисы.
По умолчанию проверки не тратят бюджет API.

## Проверка XML/gzip-каталога

```powershell
go run ./cmd/catalog-check -file "C:/path/to/catalog.xml" -max-issues 20
```

Команда автоматически распознаёт gzip по сигнатуре и пишет JSON в stdout.
Для stdin использовать `-file -`. Код выхода 0 — данные прошли проверку,
2 — завершённый разбор с диагностикой данных, 1 — ошибка/неполное чтение.
При запуске через `go run` оболочка может получить код Go-инструмента вместо
кода дочерней программы; точный код даёт собранный executable.

Проверка не записывает данные в БД и не активирует снимок. `can_activate` в отчёте
означает отсутствие ошибок reader, а не готовность поиска и не загрузку товаров
в магазин. Подробный контракт и ограничения — в [спецификации](docs/import.md).

## PostgreSQL и импорт

```powershell
# Только если .env ещё нет; существующий файл с ключом не заменять.
if (!(Test-Path .env)) { Copy-Item .env.example .env }
docker compose up -d --wait postgres
go run ./cmd/catalog-import -migrate-only
go run ./cmd/catalog-import
```

В локальном .env указать CATALOG_PATH и POSTGRES_DSN. Значения PostgreSQL в
.env.example согласованы с Compose; при смене пароля/порта обновить и DSN.
Пароль POSTGRES_PASSWORD применяется при первом создании volume: его изменение
в .env не меняет пароль уже созданной БД. Для Go-тестов ключ не требуется.
catalog-import читает .env, переменные процесса имеют приоритет. Флаги -file
и -batch-size (по умолчанию 500) позволяют выбрать файл и размер порции.

Сервис доступен только на localhost, лимит — 1 ГБ RAM и 2 CPU. Данные живут
в named volume; `docker compose stop` останавливает сервис без удаления данных.
Elasticsearch включается отдельно профилем search: 2 ГБ RAM/2 CPU, heap 512 МБ.
После импорта запустить `docker compose --profile search up -d --wait elasticsearch`,
`go run ./cmd/catalog-index`, затем `go run ./cmd/search -query "сервер"`.
Для локального PostgreSQL с лимитом 1 ГБ перед индексацией в PowerShell:
`$env:PGOPTIONS = '-c max_parallel_workers_per_gather=0 -c jit=off'`.
Это проверенная конфигурация; без неё чтение категорий полного снимка превышало 30 с.
При медленном чтении больших JSON можно индексировать оригинальный feed:
`go run ./cmd/catalog-index -local-feed <путь-к-feed>` — только при совпадении SHA,
версии нормализации и reader limits с активным снимком; [правила и измерения](docs/product-roles.md).
Индексы хранятся в собственном named volume; старые автоматически не удаляются.

После оплаченного пилота доступен эксперимент без новых API-вызовов:
`go run ./cmd/search -query "хочу находить уязвимости в сети компании" -hybrid-corpus artifacts/embedding-pilot-dataset.json`.
Кэш покрывает 512 групп; новые тексты без эмбеддинга используют BM25 fallback.
Команды измерений и ограничения — [гибридный поиск](docs/hybrid-search.md).

Полный корпус всех групп/SKU: `python scripts/prepare-vector-corpus.py --feed <путь>`,
план — `go run ./cmd/vector-index -mode plan`. После разрешённой индексации:
`go run ./cmd/search -query "сервер" -vector-corpus artifacts/vector-corpus.json`.
Новые запросы оплачиваются только при `-live-query` и OPENAI_LIVE_ENABLED=true;
общий ledger сохраняет лимит $5. Для серии запросов доступен `-batch` (JSON-строка
на строку stdin), корпус загружается один раз. См. [полный корпус](docs/vector-corpus.md).

Импорт копирует источник во временный файл data/import-cache, пишет порциями
в staging и активирует снимок только после полного успешного чтения.
Повтор той же команды продолжает запись с checkpoint, заново валидируя весь XML.
Повтор готового снимка ничего не дублирует и не откатывает active на старую версию.
Сохраняются все исходные параметры и нормализованные характеристики с evidence
по [политике characteristics-v2](docs/normalization.md). Неизвестные/противоречивые
значения остаются явными; family_id и совместимость ещё не вычисляются.
Подробности — [сохранение снимков](docs/persistence.md).

Для интеграционных тестов указать DSN локальной тестовой БД (нужен CREATE SCHEMA):

```powershell
$env:TEST_POSTGRES_DSN = 'postgres://smart_search:smart_search_local@localhost:5432/smart_search?sslmode=disable'
go test -count=1 -v ./internal/infrastructure/postgres
Remove-Item Env:TEST_POSTGRES_DSN
```

Каждый тест создаёт отдельную схему и удаляет только её; основной каталог не очищается.
В CI эти тесты обязательны через quality-gate и используют отдельный PostgreSQL.

Каталог, локальные данные и артефакты хранятся вне Git в `data/`. Большой feed
может оставаться в Downloads: достаточно указать путь.
Никакие инструкции не требуют копировать файл в репозиторий.

Общий лимит OpenAI API — **5 USD** на индексацию, эксперименты и живые проверки.
Платные операции должны быть явно включены; перед полной индексацией требуется
оценка стоимости. Приложение локальное, использование OpenAI требует интернета.
Выборочный LLM-разбор включается отдельно: `-parser-mode replay` использует кэш,
`live` требует `-live-query` и явного разрешения через окружение.
План без сети: `go run ./cmd/query-parse -mode plan -query "нужно помогать сотрудникам с компьютерами"`.
Миграция 004, команды и лимиты — [исполнение интерпретации](docs/query-parser-runtime.md).

## Данные и ограничения MVP

По [уточнениям экспертов](docs/expert-answers.md) ответ — топ-4 разных семейств:
при нехватке прямых совпадений включаются программно проверенные допродажи.
Приоритет у подходящих предложений с ценой; если их нет, допускается запасная
выдача без цены. Цена 0 неизвестна, available=false не означает «под заказ».
Достаточно снимка feed; онлайн-сбор цен не требуется. Синтетические B2B-профили
явно обозначаются. Отдельный экран аналитики остаётся выбранным бонусом.

Логи кликов/заказов и маржа не предоставлены. Это не блокирует контракты и тесты;
измеренный рост продаж и популярность без данных не заявляются.
