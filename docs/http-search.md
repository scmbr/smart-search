# Первый POST /search

Спецификация начального lexical backend через application/lexical.Search.
Текущий HTTP поддерживает hybrid/live: [конфигурация](http-hybrid.md).
Условия и повторная проверка SKU сохраняются во всех режимах.

## Приёмка

- JSON <=16 KiB, UTF-8, query <=256 символов, session_id и Idempotency-Key
  обязательны, ID по контракту. Неизвестные/повторные поля, null, лишний JSON,
  неверные типы и неподдержанные параметры отклоняются до вызова поиска.
- Поддержаны limit 4..20, availability_policy, platform, version, min_users,
  offer_kind. Пока только context anonymous/unspecified без category_id;
  cursor, synthetic/segment context и license_type дают понятный 400.
  Неразбираемые численные ограничения запроса тоже 400, без поиска.
- Повтор session/key + канонический JSON возвращает тот же search_id,
  длительность и результат, в том числе после рестарта и смены индекса.
  Другой JSON — 409. Порядок ключей/пробелы JSON несущественны; содержимое
  строк и наличие явных defaults существенны. Срок хранения не менее 24 часов.
- PostgreSQL advisory lock по session/key исключает параллельное выполнение.
  Busy/error — 503; успешный результат сохраняется до ответа. Падение процесса
  освобождает lock. Неуспешный поиск не кэшируется как успешная пустая выдача.
- Отдельный pool для idempotency locks не занимает соединения каталога/
  будущего embedding Runtime. Ошибки хранения не возвращаются как HTTP 200.
- Карточки из реального каталога, стабильные product/offer/family IDs, только
  HTTPS store.softline.ru. Нулевая цена не становится известной.
- family_confirmed описывает идентичность семейства, не релевантность/лицензии.
  top4 содержит только подтверждённые семейства среди показанных matches;
  недобор объясняется shortfall_reason. Provisional карточки могут быть в matches,
  но не доказывают выполнение требования четырёх разных семейств.
- recommendations/alternatives пока пусты, next_cursor=null; corrections и
  clarification поддержаны [именованным поиском](named-search.md).
  total_matches=null, если точный глобальный total неизвестен; для пустой выдачи 0.
  Это уточнение контракта 0.3.0, а не число кандидатов под видом полного count.
- Ошибки 400/409/413/503 безопасны; deadline запроса 30 секунд. События аналитики
  ещё не реализованы; таблица сохранённых поисков не выдаётся за готовую аналитику.

Миграция 005 добавляется forward-only через `go run ./cmd/catalog-import -migrate-only`.
Сервер сам миграции не запускает. После применения: `go run ./cmd/api`.
Остальные endpoints (products, suggest, events, trace, analytics) ещё не реализованы.

Пример из PowerShell:

```powershell
$body = @{ query = 'AnyDesk'; session_id = 'demo-session'; limit = 4 } | ConvertTo-Json
Invoke-RestMethod -Uri http://127.0.0.1:8080/api/v1/search -Method Post -ContentType 'application/json' -Headers @{ 'Idempotency-Key' = 'demo-request-1' } -Body $body
```

Новый запрос в той же сессии использует новый Idempotency-Key. Повтор прежнего
ключа предназначен для воспроизведения сохранённого поиска, не обновления цен.

## Фактическая проверка

Real PG тест на собственной схеме проверил lock exclusion, persistent replay
через новый pool, конфликт тела, session scope и expiry. Тесты application
проверяют replay без engine, ошибки engine/storage и отмену без сохранения.
HTTP/контрактные проверки покрывают фактические success/empty/error ответы,
Unicode canonicalization, точные закрытые поля, повторы ключей, body limit
и исключение provisional family из top4.

После применения migration 005 отдельный bin/api.exe вернул AnyDesk SKU 402265.
Тот же ключ/JSON с другим порядком полей дал идентичный ответ. После перезапуска
с недоступным ES replay остался идентичным; новое обращение дало 503,
изменённое тело — 409, oversized — 413, отрицательный запрос — 200/пустой список.
Локальный отчёт artifacts/http-search-smoke.json; процессы остановлены.
Duration фиксирует исполнение engine; replay сохраняет прежнее значение.

## Гибридный режим

Подключён отдельной конфигурацией сервера: [http-hybrid.md](http-hybrid.md).
По умолчанию lexical; включение не меняет JSON-запрос или правила idempotency.
