# Семейства полного каталога: families-v4

## Приёмка

Редакции OfficeSuite Business, Business Extra, Family, Personal и Home & Business
занимают одно семейство. Подписка, lifetime и число пользователей остаются
характеристиками SKU; объединение не подтверждает одинаковые права лицензий.

RuDesktop, АССИСТЕНТ и LiteManager — три самостоятельных семейства.
EdrawMax, EdrawMind и EdrawProj — три самостоятельных продукта одного вендора.
Неизвестные группы этих вендоров остаются provisional; чужой vendor scope
никогда не получает подтверждение по совпадению group_id.

Источник групп и SKU — полный feed с зафиксированным SHA. Каталожный аудит
проверяет все предложения каждой добавленной группы и сохраняет примеры.
Это структурное доказательство, не разметка релевантности или совместимости.
Новые продукты MobiOffice/MobiPDF и другие продукты Wondershare не добавляются
по сходству названия или принадлежности вендору.

До изменения правил: unit-тесты границ семейств и real Elasticsearch-тест
collapse для нескольких OfficeSuite SKU в lexical и semantic resolve.
После: аудит feed, новый физический индекс и reuse, cache-only tuning/probes.
Нормализация, документ lexical-v6, запрос query-v5 и тестовые цели не меняются.
Holdout не запускать; платных API-вызовов эта задача не требует.

## Дополнительные источники границ

- [Wondershare: отдельные EdrawMax, EdrawMind, EdrawProj](https://edrawsoft.wondershare.com/about-us.html).
- [RuDesktop: удалённый доступ](https://rudesktop.ru/solutions/udalennyj-dostup-i-podderzhka/).
- [LiteManager: модули одного продукта](https://litemanager.ru/support/help_ru/use_litemanager/).

Исполняемые правила используют ссылки на реальные SKU из frozen evidence;
внешние страницы не подменяют программную проверку характеристик.

## Аудит снимка

| Семейство | Группы | Предложений |
| --- | --- | ---: |
| OfficeSuite | 300944, 326066, 326067, 326068, 341691 | 5 |
| RuDesktop | 360667 | 3 |
| АССИСТЕНТ | 120894 | 12 |
| LiteManager | 46113 | 8 |
| EdrawMax | 301708 | 40 |
| EdrawMind | 301736 | 30 |
| EdrawProj | 301233 | 5 |

Итого добавлено 7 семейств, 11 групп, 103 предложения. Полный маппинг:
57 семейств, 106 групп, 6 360 предложений из 136 040. Повторный аудит всех
групп совпал с testdata/evaluation/family-evidence.json. SHA feed:
27eaec9e77942c61d58b3c04dcaa8991c707dc0fcb64a6442a7408d9908cb148.
Остальные предложения не становятся подтверждёнными автоматически.

## Проверка индекса

Индекс smart-search-ec3c31d8c98f37555e04d660c496a85f7765b35df32519e8cb92dbab7f6390ff
содержит все 136 040 документов. Количество документов каждого из 57 семейств
и 106 групп точно совпало с frozen evidence: 6 360 подтверждённых предложений.
Сборка через verified_feed — 234 184 мс, повтор обычным PostgreSQL-путём —
669 мс, Reused=true. Приватные временные файлы удалены автоматически.
Предыдущий индекс сохранён; active snapshot не изменён.

Family version: families-v4-f9f4cc47eda0fec86f6021b9bf7a01435e09f7cfaa6356cc81c57278e0b55c88.
Unit-тесты границ, реальный ES collapse, domain/application, API-контракт,
vet, build и восемь Python-тестов прошли.

Cache-only tuning: 52/52 групп, 6/6 брендов, 2/2 отрицательных — те же SKU
в той же последовательности, что до изменения. Frozen development probes:
7/16 без изменения метрики. Только probe-16 изменил состав top4: вместо
OfficeSuite Family/Personal появились MobiOffice и DocSpace. MobiOffice пока
provisional: граница старого/нового продукта не подтверждена в этой версии.
FourFamiliesConfirmed=false сохраняется; релевантность всех четырёх не доказана.
Цели/наборы не менялись, holdout не запускался, OpenAI не вызывался.

Локальные отчёты: artifacts/families-v4-evidence-check.json,
families-v4-index.json, families-v4-index-replay.json, families-v4-coverage.json,
families-v4-tuning.json и families-v4-probes.json; в Git не включены.
