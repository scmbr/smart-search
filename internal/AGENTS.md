# Backend: Go и Clean Architecture

Область: `internal/`, а по указанию корня также `cmd/`, `contracts/` и Go-конфигурация.
Все команды ниже запускаются **из корня репозитория**; go.mod находится там.

## Реальные команды и состояние

| Задача | Команда |
| --- | --- |
| Зависимости | `go mod download`; после изменения импортов — `go mod tidy` |
| Запуск текущей CLI | `go run ./cmd/catalog-check -file testdata/catalog/minimal.xml` |
| PostgreSQL | `docker compose up -d --wait postgres` |
| Миграции (forward-only) | `go run ./cmd/catalog-import -migrate-only` |
| Импорт из CATALOG_PATH | `go run ./cmd/catalog-import` |
| Elasticsearch + PostgreSQL | `docker compose --profile search up -d --wait postgres elasticsearch` |
| Индексация активного снимка | `go run ./cmd/catalog-index` |
| Поиск CLI | `go run ./cmd/search -query "сервер"` |
| Диагностика только tuning | `python scripts/evaluate-tuning.py` после сборки bin/search.exe |
| Интеграционные тесты всех адаптеров | задать `TEST_POSTGRES_DSN` и `TEST_ELASTICSEARCH_URL`, затем `go test -count=1 -v ./internal/infrastructure/...` |
| Интеграционные тесты | задать `TEST_POSTGRES_DSN`, затем `go test -count=1 -v ./internal/infrastructure/postgres` |
| Сборка CLI | `go build -o bin/catalog-check.exe ./cmd/catalog-check` |
| Форматирование: проверка / исправление | `gofmt -l internal cmd contracts` / `gofmt -w internal cmd contracts` |
| Статический анализ | `go vet ./...` |
| Проверка типов и компиляции | `go build ./...` |
| Все тесты | `go test ./...` |
| Покрытие | `go test -count=1 -cover ./...` |
| Целостность модулей | `go mod verify` |

`gofmt -l` должен дать пустой вывод. golangci-lint/отдельный task runner не настроены.
HTTP-сервера пока нет. `catalog-import` читает .env без замены переменных среды,
применяет встроенные SQL-миграции и пишет staging/active снимки. `catalog-check`
только читает/проверяет файл. Не передавай DSN с паролем через аргументы команд.
Тесты PostgreSQL создают/удаляют отдельные схемы; обычные go test их пропускают,
CI запускает их с отдельным сервисом и требует успех через quality-gate.
Elasticsearch-тест создаёт только собственный индекс/alias со случайным ID;
без TEST_ELASTICSEARCH_URL пропускается. Индексы/volumes пользователя не удаляй.
CLI search/catalog-index используют порты application; правила baseline —
docs/lexical-baseline.md. Маппинг семейств — internal/infrastructure/families/rules.json;
его канонический SHA-256 входит в index version. Новая семантика документа требует
повышения DocumentSchemaVersion; provisional family keys не означают разные семейства.
Query policy — docs/query-normalization.md и docs/query-constraints.md, версионируется QueryPolicyVersion отдельно
от индекса. Не расширяй цифры/артикулы fuzzy; фильтры сохраняются во всех попытках.
Разметка 100 кейсов принята пользователем; не запрашивай это одобрение повторно.
Не запускай holdout для подбора правил; evaluate-tuning.py читает только tuning.
CLI-сборка выше рассчитана на Windows; на другой ОС используй исполняемое имя без `.exe`.
CI запускает форматирование, vet, сборку и тесты на Linux и Windows при наличии
отслеживаемого go.mod. Локальные проверки обязательны независимо от CI.

## Слои и направление зависимостей

Жёсткое правило: **Infrastructure / Presentation → Application → Domain**.
Внешние слои могут использовать domain, но внутренние никогда не импортируют внешние.
Структура нового импорта уже использует domain/application/infrastructure;
presentation появится вместе с HTTP API:

- `internal/domain/`: сущности, Value Objects, бизнес-инварианты, доменные ошибки
  и интерфейсы репозиториев. Только стандартная библиотека; никаких ORM, SQL,
  HTTP/XML, SDK, транспортных DTO и импорта других слоёв.
- `internal/application/`: use cases, входные/выходные DTO и порты внешних сервисов.
  Из проектных слоёв зависит только от domain; SDK/SQL/XML/HTTP сюда не проникают.
- `internal/infrastructure/`: реализации репозиториев, PostgreSQL/Elasticsearch/OpenAI,
  файловые/XML/gzip-адаптеры, конфигурация и реализации внешних портов.
- `internal/presentation/`: handlers, транспортная валидация и маппинг HTTP ↔ DTO;
  никаких бизнес-решений и прямого обхода use cases через БД/SDK.
- `cmd/<command>/`: composition root — создаёт адаптеры, внедряет их в use cases,
  подключает handlers и управляет жизненным циклом. Внутренние пакеты не импортируют cmd.

Сейчас есть `internal/catalog/` — reader/валидатор со смешанной нормализацией и XML,
а не готовый domain-слой. `cmd/catalog-check/` вызывает его напрямую.
Не называй текущую структуру уже соответствующей Clean Architecture. По мере
развития каталога выделяй бизнес-инварианты внутрь, reader оставляй адаптером;
массовое перемещение существующего кода требует отдельной задачи.

## Новая фича

1. Уточни сценарий и критерии приёмки в спецификации, выбери границы контекста.
2. Опиши domain-модель/ошибки/репозиторный порт и сначала напиши unit-тесты инвариантов.
3. Добавь application use case, DTO и необходимые сервисные порты; unit-тестируй
   успех, ошибки, отмену и пограничные условия с test doubles, без сети/БД.
4. Реализуй infrastructure-адаптеры с явным маппингом внешних данных; добавь
   интеграционные проверки отдельно от unit-тестов.
5. Добавь тонкий presentation handler, затем собери зависимости в cmd.
6. Сверь изменения с общим API-контрактом; выполни команды проверок выше.

Unit-тесты domain и application обязательны; тест схемы/fixture их не заменяет.
Интерфейсы держи маленькими. Внутрь передавай DTO/доменные значения, не pgx-строки,
XML-токены, HTTP request/response или структуры OpenAI/Elasticsearch SDK.

## Обработка каталога и внешних вызовов

- Передавай context, ограничивай тайм-ауты, повторы и число параллельных задач.
- Читай XML/gzip потоком. Sink пишет в staging: поздняя ошибка инвалидирует снимок;
  callback и CanActivate не означают завершённую транзакцию или активный каталог.
- Храни деньги в целых копейках, неизвестные значения явно; не угадывай версии,
  роли лицензий и family_id по одной группе.
- Характеристики вычисляет domain.Normalize в application импорта; правила —
  docs/normalization.md (characteristics-v5). Меняя семантику/маппинг, повышай NormalizationVersion:
  она включена в identity снимка. Старые ready-снимки не переписывай.
- Требования поиска/допродаж — в `docs/search.md`, `docs/verification.md` и
  `docs/expert-answers.md`. Проверяй характеристики выбранных SKU программно;
  ссылки и ответ LLM не заменяют passed по обязательным правилам.
