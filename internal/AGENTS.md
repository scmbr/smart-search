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
| Индексация из сверенного feed | `go run ./cmd/catalog-index -local-feed <путь>`; SHA/normalization/limits должны совпасть с active snapshot |
| Поиск CLI | `go run ./cmd/search -query "сервер"` |
| Экспериментальный гибрид без API | `go run ./cmd/search -query "хочу находить уязвимости в сети компании" -hybrid-corpus artifacts/embedding-pilot-dataset.json` |
| Диагностика только tuning | `python scripts/evaluate-tuning.py` после сборки bin/search.exe |
| Корпус пилота | `python scripts/prepare-embedding-pilot.py --feed <путь>` |
| План расходов / повтор из кэша | `go run ./cmd/embedding-pilot -mode plan` / `-mode replay` |
| Полный корпус только из feed | `python scripts/prepare-vector-corpus.py --feed <путь>` |
| Полный план / проверка кэша | `go run ./cmd/vector-index -mode plan` / `-mode replay` |
| План интерпретации без сети | `go run ./cmd/query-parse -mode plan -query "нужно помогать сотрудникам с компьютерами"` |
| Кэшированный parser | `go run ./cmd/search -query "нужно помогать сотрудникам с компьютерами" -vector-corpus artifacts/vector-corpus.json -parser-mode replay` |
| Development-план parser | `python scripts/evaluate-query-parser.py` после сборки bin/query-parse.exe |
| Поиск по полному корпусу | `go run ./cmd/search -query "сервер" -vector-corpus artifacts/vector-corpus.json` |
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
Роли SKU — docs/product-roles.md и docs/capacity-extensions.md: lexical-v7,
product-role-v2/query-v7. Дополнительные IP/узлы/устройства — addon;
количество дополнительных единиц и узлов пока unsupported, не суммировать с базой.
Назначение — docs/product-purpose.md: purpose-v1, явный запрос ОС/замены AD,
vendor/group + условия SKU до collapse и повтор в application; unknown не равен
доказанному назначению. Purpose использует прежние поля; роли v2 требуют нового
индекса lexical-v7. Нормализация снимка characteristics-v5 не менялась.
Классифицируй по идентичности/источникам, а не по query targets. Фильтруй до
collapse во всех ветках; application пересчитывает роль/evidence. Unknown не
является подтверждённым основным продуктом. Не исключай SaaS по слову «сервис»
или полную лицензию с включённой поддержкой. После изменения классификатора
повышай версию документа и перестраивай отдельный индекс, без удаления старого.
Файловый источник индекса не обходит pinning/validation/current check: приватная
копия и spool <=4 ГиБ, полная проверка до Prepare, тот же domain.Normalize и
проверенное равенство Document. Не импортируй изменённый файл под старым snapshot ID.
Разметка 100 кейсов принята пользователем; не запрашивай это одобрение повторно.
Не запускай holdout для подбора правил; evaluate-tuning.py читает только tuning.
Пилот OpenAI — docs/embedding-pilot.md. Его live-режим требует явного разрешения
на отправку корпуса и OPENAI_LIVE_ENABLED=true; текущее разрешение записано D-031.
Ключ не выводить. Вызовы проходят через api_budget/api_reservations и embedding_cache
(миграция 002); не обнулять расходы/неизвестные резервы при рестарте или ошибке.
Новые платные компоненты должны учитывать этот же общий бюджет, а не создавать
отдельный лимит 5 USD. Эмбеддинги исполняй через application/embeddingpilot.Runtime:
общие слоты/ключи и постоянные pending migration 003; см. docs/embedding-runtime.md.
Не обходи Runtime прямым вызовом Provider/SDK. Не освобождай unresolved claims
по TTL и не повторяй тайм-ауты/5xx автоматически. HTTP fallback пока не подключён.
CLI-сборка выше рассчитана на Windows; на другой ОС используй исполняемое имя без `.exe`.
Гибрид — docs/hybrid-search.md; кэшированный development-корпус не означает полное
покрытие. RRF выполняется по FamilyID после фильтрации SKU; не обходи общий
application/lexical.Search при выдаче результата. Новые API-тексты требуют отдельного
live-подключения и общего ledger, отсутствие кэша сейчас даёт явный BM25 fallback.
Полный manifest — docs/vector-corpus.md; разрешение пользователя — D-034.
Для corpus-index live обязательны approved-sha и OPENAI_LIVE_ENABLED=true;
предел запуска $0.15 поверх прежнего Runtime cap $0.05 на вызов, общий ledger $5.
Новый запрос оплачивается только с -live-query, максимум $0.001 на запрос.
Без group_id ищи точный OfferID с vendor-path; не придумывай группу.
Полный корпус не загружается частично; prefix-512 сохраняется в памяти процесса.
Для development evaluation используй --vector-corpus <путь> --batch: это cache-only,
не добавляй live-флаг и не используй holdout для настройки.
Parser — docs/query-parser-runtime.md: replay без Provider, live только явным
режимом и OPENAI_LIVE_ENABLED=true; $0.001 на интерпретацию, без retry. Общие
ledger/API-слоты с эмбеддингами; migration 004, dimensions=0 для текста.
Неизвестный исход не очищай. Невалидный оплаченный ответ кэшируется как неудача.
Модель/prompt/schema/лимиты версионируй; не меняй молча кэш или тариф.
Для диагностики с parser используй evaluate-tuning.py --vector-corpus <путь>
--batch --parser-replay; live parser в batch запрещён, development-run отдельно
планируется evaluate-query-parser.py с fingerprint и лимитом $0.02.
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
