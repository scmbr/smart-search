# Переименование OfficeSuite: families-v5

## Требование и границы

OfficeSuite и MobiOffice занимают одно место семейства в lexical, semantic
resolve и RRF. Стабильный ID mobisystems-officesuite сохраняется; отображаемое
название — MobiOffice (OfficeSuite). Добавляется только проверенная группа
397581 в scope /mobilesystems/. Пять прежних групп остаются в том же семействе.

MobiPDF (группа 393014) — отдельное семейство mobisystems-mobipdf. MobiDocs,
MobiSheets, MobiSlides, MobiMail, Aqua Mail, File Commander и неизвестные
группы не объединяются по вендору или общему префиксу Mobi.

Общее семейство не означает взаимозаменяемые лицензии, бесплатное обновление,
подтверждённые права, одинаковые платформы или актуальную поддержку.
Сведения о SKU/ценах берутся из feed; онлайн-данные не заменяют снимок.

## Основания, проверены 2026-11-28

- [MobiSystems: переименование OfficeSuite в MobiOffice, PDF Extra в MobiPDF](https://mobisystems.com/en-gb/renew).
- [Анонс 2024-11-04: преемственность пакета и отдельные приложения](https://mobisystems.com/en-us/blog/news/mobisystems-unifies-office-apps-and-launches-mobiscan).
- [FAQ продукта: новый MobiOffice и старый OfficeSuite](https://mobisystems.com/en-us/mobioffice).

Это проверенные сведения о границе продуктов, а не результаты измерения
релевантности. SKU и все предложения добавленных групп подтверждаются отдельным
аудитом того же feed и testdata/evaluation/family-evidence.json.

## Приёмка до реализации

1. Unit-тест: шесть групп офисного пакета дают один подтверждённый ID;
   MobiPDF даёт другой ID; неверный scope и неизвестные соседи provisional.
2. Real ES: lexical-запрос по обоим названиям и semantic resolve по обеим
   группам дают одно место; добавленный PDF-продукт — второе, не четвёртое.
3. Точные SKU и фильтры выбирают конкретное предложение, несмотря на collapse.
4. Аудит всего маппинга, новый индекс 136 040, точные counts и reuse.
5. Cache-only tuning/development; holdout и платные API не используются.

Нормализация characteristics-v5, документ lexical-v6, query-v5 и корпус
эмбеддингов не меняются. Новый family fingerprint требует отдельного индекса.

## Аудит feed

Группа 397581: 5 предложений MobiOffice; вместе с прежними редакциями
OfficeSuite семейство содержит 10 предложений. Группа 393014: 9 предложений
MobiPDF. Полный маппинг — 58 семейств / 108 групп / 6 374 предложения.
Повторный аудит всех групп совпал с frozen evidence; vendor scope проверен
у каждого SKU. Другие группы MobiSystems остались provisional.

SHA исходного feed:
27eaec9e77942c61d58b3c04dcaa8991c707dc0fcb64a6442a7408d9908cb148.

## Проверка индекса и тестов

Полный индекс smart-search-15c2e81461416db9126ce03de7aba745ec2bc0be4be4f5f624491ef4ed9eea52:
136 040 документов, counts каждого из 58 семейств / 108 групп точно совпали
с frozen evidence. Семейства подтверждены для 6 374 предложений.
Сборка verified_feed — 223 824 мс; повтор PostgreSQL-путём — 96 мс,
Reused=true. Старый индекс сохранён, snapshot не менялся.
Family version: families-v5-92f4b076162ce0670dabf3a5cbc34a5fe0ca5db8396e61671e0d3c656b48786c.

Unit-тесты границ, real ES collapse/resolve/точный SKU, офлайн go test ./...,
vet, build, форматирование, восемь Python-тестов и сверка таблицы 100 кейсов
прошли. В synthetic ES fixture версия 2025 задана явным параметром «Версия»;
произвольный год в имени MobiOffice не считается доказательством версии.

Cache-only tuning: прежние 52/52 групп, 6/6 брендов, 2/2 отрицательных;
все последовательности SKU совпали с families-v4. Development probes — 7/16.
В probe-16 MobiOffice больше не занимает второе место того же семейства:
теперь SKU 473755, 473750, 475126, 417988. Последние кандидаты provisional,
в том числе aytoSchema Lite; релевантность всех четырёх не доказана,
FourFamiliesConfirmed=false. Это исправление идентичности, не рост метрики.

Lexical smoke на полном feed: MobiOffice → 481907, OfficeSuite → 532545,
MobiPDF → 475569; точный 481907 → тот же SKU. Каждый ответ содержит одно
подходящее семейство, без заполнения четырёх мест случайными товарами.
Holdout не запускался, OpenAI не вызывался; тестовые цели не менялись.

Локальные отчёты: artifacts/families-v5-evidence-check.json,
families-v5-index.json, families-v5-index-replay.json, families-v5-coverage.json,
families-v5-tuning.json, families-v5-probes.json, families-v5-named-smoke.jsonl.
Они не включены в Git.
