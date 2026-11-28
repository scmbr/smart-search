# HTTP hybrid: явное включение

По умолчанию API использует lexical. `-vector-corpus <manifest>` включает hybrid-v4
с прежними eligibility-фильтрами и повторной проверкой application/lexical.Search.
Корпус должен соответствовать активному нормализованному feed; неверный manifest
или несовпадение при старте — ошибка конфигурации. Смена каталога после запуска
означает lexical fallback. Клиент не управляет платными режимами.

`-live-query` разрешает эмбеддинги новых запросов только вместе с корпусом,
OPENAI_LIVE_ENABLED=true и ключом. `-parser-mode off|replay|live`: off по умолчанию;
replay не создаёт Provider, live требует live-query и проверки модели/бюджета.
Общие постоянные Runtime/ledger, $5 на проект, $0.001 на новый запрос и отдельно
$0.001 на интерпретацию; без автоматического повтора неизвестного исхода.

Корпус прогревается из embedding_cache при запуске без Provider и без оплаты.

Live network smoke выявил, что 2s optional timeout недостаточен: завершённый
embedding-вызов занял 9.20s по ledger. Теперь vector port ограничен 15s, parser —
10s, общий поиск — 30s, HTTP write — 35s. Более короткая отмена клиента сохраняется.
Бюджеты и запрет повтора неизвестного исхода не меняются. Кэшированные запросы
и поиск названий не ждут эти лимиты при нормальной работе.
Ошибка прогрева не отключает lexical: optional vector/parser операции ограничены
собственными тайм-аутами внутри общего HTTP deadline. Отмена родительского запроса
не превращается в успешный fallback. Основной ES недоступен — 503, не ложная пустая выдача.

Ответ сохраняет существующую схему: mode=hybrid при успешной векторной ветке,
иначе lexical. Vector fallback/catalog mismatch → embeddings_not_ready;
parser fallback/invalid → openai_unavailable (включая отсутствие replay-кэша,
это недоступность функции, не доказательство сетевого сбоя). Отсутствие соседей,
точный артикул и выключенный parser не считаются деградацией. clarify/unsupported
дают безопасную общую подсказку без выдачи внутреннего LLM rewrite.

Сохранённый idempotent ответ сохраняет прежние mode/degraded_reasons после
смены конфигурации и рестарта, без нового engine/API-вызова. Readiness остаётся
проверкой обязательного PG/ES; optional OpenAI не блокирует health.

Приёмка: тесты комбинаций конфигурации, transport mode/degraded/clarification,
тайм-аутов optional портов и parent cancellation; существующие hybrid/runtime и
persistent replay тесты; реальный cache-only HTTP proxy smoke без платных вызовов.

## Запуск

Из корня, после миграций и подготовки corpus/cache:

```powershell
go run ./cmd/api -vector-corpus artifacts/vector-corpus.json -parser-mode replay
```

Такой запуск не обращается к OpenAI: незакэшированный запрос даёт lexical fallback.
Для явно разрешённого live запуска добавить `-live-query -parser-mode live`,
предварительно установить OPENAI_LIVE_ENABLED=true в среде backend. Ключ остаётся
в .env/backend. Файл .env автоматически не изменяется. Новые запросы могут
расходовать общий бюджет; parser вызывается только селективной политикой hybrid.
Порог и качество остаются hybrid-v4; подключение HTTP само по себе не улучшает метрики.

## Историческая проверка cache-only

На порту 8081 corpus/replay smoke: task probe-01 → hybrid, четыре карточки;
«виндовс» → lexical/Windows 11 Pro с embeddings_not_ready (query cache отсутствует);
новый неизвестный текст → lexical/empty с той же диагностикой; FQC-10572 → lexical,
без деградации. Повторы идентичны до и после перезапуска в lexical режиме.
Ledger до/после: 478 reservations, actual 25425085 nanoUSD, unresolved=0.
Новые платные вызовы и holdout не запускались. Live network smoke не выполнялся.
Это проверка интеграции, а не доказательство релевантности всех четырёх карточек.
Локальный raw report — artifacts/http-hybrid-smoke.json (не коммитится).

Компактный отчёт: [http-hybrid-smoke-v1.json](../testdata/evaluation/http-hybrid-smoke-v1.json).

## Live стенд 2026-11-28

По явному запросу пользователя локальный OPENAI_LIVE_ENABLED включён в .env
(файл игнорируется Git). Команда:

```powershell
go run ./cmd/api -name-catalog artifacts/name-catalog.json -vector-corpus artifacts/vector-corpus.json -live-query -parser-mode live
```

Первый HTTP-запрос «сервис для редактирования фото» был отменён прежним 2s timeout;
его неизвестный исход сохранён, автоматического повтора/сброса claims нет.
Другой запрос через CLI подтвердил доступность OpenAI: embedding завершён за
9.20s по ledger. После изменения лимитов новый HTTP-запрос «нужна программа для
редактирования фотографий» вернул hybrid без деградации за 5.06s; replay идентичен.
Ledger: 482 reservations, actual 25481605 nanoUSD, unresolved=1 с upper=1460 nanoUSD.
Общий cap $5, прежние $0.001/query и $0.001/interpretation сохранены.

Это проверка live-интеграции, не улучшение ранжирования: top4 — Автограф, PhotoGrav,
ТИМ КРЕДО ФОТОГРАММЕТРИЯ, ФотоШОУ PRO. Photoshop был среди CLI vector-кандидатов,
но не попал в итоговый top4. Исправление семантического ранжирования остаётся задачей.
Локальные отчёты — artifacts/live-http-smoke.json, artifacts/live-photo-cli.json.
