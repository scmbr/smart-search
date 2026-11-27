# Исполнение интерпретации

SDD: interpretation-v1 подключается к GPT-6 Luna через официальный SDK и отдельный
application use case. Только исходный запрос и фиксированные prompt/schema;
каталог, цели tuning/holdout, цены и ключи в prompt не передаются.

## Инварианты

- По умолчанию off; replay читает постоянный кэш без Provider. Live требует
  явного режима, OPENAI_LIVE_ENABLED=true и ключа. Автоматических повторов нет.
- Ключ SHA-256 включает model, prompt version, interpretation version и точный
  исходный текст. Перезапуск не сбрасывает кэш, резерв или pending claim.
- Один запрос: максимум 512 выходных токенов, 30 секунд, $0.001. Верхняя оценка
  входа — UTF-8 bytes prompt/schema/query + 1024 на framing. До сети резервируем
  эту оценку и весь max_output. Непоместившийся резерв запрещает вызов.
- Тариф: стандартный GPT-6 Luna $0.10/M input, $0.50/M output; cache writes
  $0.125/M input. Для ledger консервативно считаем весь input по $0.125/M,
  без скидки за кэш. Это учтённая верхняя стоимость по usage, не счёт OpenAI.
- Используется прежняя api_budget/api_reservations и её транзакционная блокировка:
  общий $5 с эмбеддингами. Отдельного бюджета нет. Два общих API-слота и ключевые
  блокировки используются вместе с embedding runtime; pool=3 не должен зависать.
- Миграция 004 только добавляет output_tokens, parser cache и parser pending;
  миграции 001–003 не меняются. dimensions=0 в ledger обозначает текстовый ответ.
- До вызова сохраняем pending. Тайм-аут, 5xx, отсутствие/невалидность usage или
  сбой settlement оставляют резерв/claim: повтор после рестарта запрещён.
- Явный 4xx (кроме 408) снимает claim, сохраняет консервативный резерв. Нет
  автоматического retry даже для 429. Сырые SDK-ошибки не выходят наружу.
- При корректном usage malformed JSON/refusal/truncation учитываются как расход
  и кэшируются как безопасная неудача без сырого ответа. Replay не оплачивает их
  снова. Успешный ответ дополнительно проверяет domain.ValidateInterpretation.
- Atomic settlement: точный reservation/pending key, actual<=upper, usage bounds,
  ledger/cache/удаление claim в одной транзакции. Отмена клиента после ответа
  не мешает краткой попытке settlement с отдельным timeout.

## Критерии приёмки

Unit: cache-only, cache corruption, успех/replay, недостаточный лимит,
invalid/refusal/truncation, invalid usage, timeout, отмена, ошибки store.
SDK httptest: строгая схема, store=false, фиксированные model/tier/token limit,
нет tools, нет retry, redaction, usage/finish/refusal. PostgreSQL: shared budget,
slots, конкурентный cache miss, pending после рестарта, rollback settlement.
CLI: план без API, явные replay/live; batch evaluation допускает только replay.

Источники тарифа и возможностей (проверены 2026-11-27):
[GPT-6 Luna](https://developers.openai.com/api/docs/models/gpt-6-luna),
[Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs).
Платное development-измерение проводится отдельно после офлайн-проверок и оценки
полного лимита запуска. Независимый holdout для настройки не используется.

## Команды

Пользователь явно разрешил 16 запросов этого плана и новые запросы при явно
включённом live-parser: D-037. Разрешение не требует повторного подтверждения
каждого запроса в этих пределах. Ключ и .env не изменяются командами эксперимента.

Из корня, после установки зависимостей:

```powershell
go run ./cmd/catalog-import -migrate-only
go build -o bin/query-parse.exe ./cmd/query-parse
go build -o bin/search.exe ./cmd/search
go run ./cmd/query-parse -mode plan -query "нужно помогать сотрудникам с компьютерами"
python scripts/evaluate-query-parser.py
```

Последняя команда не обращается к OpenAI и печатает SHA плана 16 frozen probes.
Live-запуск требует OPENAI_LIVE_ENABLED=true в окружении процесса и точного SHA:

```powershell
python scripts/evaluate-query-parser.py --mode live --approved-sha <SHA-плана> --output artifacts/query-parser-live.json
python scripts/evaluate-query-parser.py --mode replay --output artifacts/query-parser-replay.json
python scripts/evaluate-tuning.py --vector-corpus artifacts/vector-corpus.json --batch --parser-replay --probes --output artifacts/parser-probes.json
```

Replay не получает Provider. Для уже интерпретированного запроса его новый
semantic_query тоже должен иметь эмбеддинг в кэше; он прогревается отдельным
явным `search -vector-corpus ... -live-query -parser-mode replay -query ...`.
Сам evaluate-tuning.py никогда не включает live. Для обычного интерактивного
поиска доступны `-parser-mode off|replay|live`; live требует `-live-query`
и запрещён вместе с `-batch`. Эмбеддинг и parser имеют отдельные $0.001 caps
на запрос, но один общий ledger $5. Отсутствие кэша не считается успешной проверкой.

## Измерение 2026-11-27

Полный корпус: 12 538 групп/SKU, SHA
`25bac9e399ea4b9bd1a3b11d70dc9d054e33f17ab44655c8b47ae0e9bf8b855e`;
characteristics-v5 / lexical-v5 / query-v4 / hybrid-v2, prefix-512, cosine>=.45.
План 16 probes: SHA
`d6b6995b9a9e65c4ba83c6b833b0a8153ea07239b2ff1b232e2313d4d773a432`,
верх $0.012504125 при cap $0.02. Все 16 ответов resolved и проходят контракт.
Повтор — 16 cache hits, 0 LLM-вызовов. Prompt/schema после измерения не менялись.

| Операция | Новые API-запросы | Input / output tokens | Учтено, USD |
| --- | ---: | ---: | ---: |
| LLM, frozen 16 probes | 16 | 4 208 / 897 | 0.000974500 |
| LLM, дополнительный smoke | 1 | 267 / 60 | 0.000063375 |
| Эмбеддинги новых формулировок | 11 | 326 / — | 0.000006520 |
| Всего добавлено | 28 | — | 0.001044395 |

LLM-стоимость в таблице консервативно считает весь input по cache-write rate,
а не является точным списанием со счёта. Общий ledger после запуска:
$0.024811955 из $5; 406 запросов (389 embedding + 17 LLM), unresolved=0,
parser cache=17, embedding cache=12 045. Все прежние расходы сохранены.
Live-флаги включались только в дочернем процессе; .env не изменялся.

Качество только на development:

| Набор | Без parser | С parser replay |
| --- | ---: | ---: |
| Tuning, известные положительные группы | 52/52 | 52/52 |
| Tuning, brand scope | 6/6 | 6/6 |
| Tuning, отрицательные пустые ответы | 2/2 | 2/2 |
| Frozen probes, известные положительные группы | 5/16 | 7/16 |

Два новых попадания — поиск уязвимостей и виртуальная инфраструктура.
13 из 16 probes прошли выборочный parser: два коротких запроса и один с BM25
были пропущены политикой отбора. Остальные три интерпретации проверены отдельно
через query-parse, но не влияли на итоговую выдачу. Время batch — 15.820 с на
16 probes, 15.249 с на 60 tuning, включая загрузку корпуса и startup, не p95.

Дополнительный запрос «нужно удалённо управлять компьютерами сотрудников и
помогать им с настройками» раньше выдавал SNMP-карты управления ИБП. Теперь
интерпретирован как «Программа для удалённого рабочего стола и технической
поддержки сотрудников»: AnyDesk top-1, затем Octagram, RuDesktop, САФИБ АССИСТЕНТ.
Replay возвращает те же Hits; отдельный query-parse replay — cache hit, 0 requests,
ledger до/после неизменен. Это подтверждает улучшение top-1 на одном smoke,
но не релевантность или совместимость всех четырёх позиций.

Сохраняются ложные альтернативы: библиотеки/компоненты вместо пользовательского
продукта, дополнения CSP вместо основной лицензии, управление Active Directory
вместо альтернативы каталогу, облачные компоненты вместо ОС. Некоторые другие
продукты могут быть подходящими альтернативами, хотя отсутствуют среди известных
положительных целей. 7/16 не является полной оценкой всех вариантов. Следующий
шаг — проверка назначения/ролей по каталогу и подтверждённых семейств, а не замена
целей или подгонка prompt под ожидаемые бренды. Holdout не запускался.

Локальные артефакты игнорируются Git: query-parser-{plan,live,replay}.json,
parser-{probes,tuning}.json, parser-warm-search.jsonl, parser-smoke-{live,replay,accounting}.json.
