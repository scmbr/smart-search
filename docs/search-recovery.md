# Запасной режим, версии и точные идентификаторы

## Приёмка

- Ошибка lexical/Elasticsearch не превращается в успешный пустой ответ и не
  запускает векторы вместо недоступного обязательного каталога.
- Ошибка/нет кэша векторов, ошибка semantic resolve и несовпадение корпуса
  возвращают исходную проверенную BM25-выдачу с trace; priced/unpriced,
  ограничения SKU и snapshot сохраняются. Ошибка не разрешает снять фильтры.
- Отмена запроса останавливает следующие внешние вызовы и не возвращает успех.
- Elasticsearch timeout, failed shards, неверные/отсутствующие поля ответа
  отклоняются. Несовместимые версии и повреждённая identity индекса отклоняются
  до чтения товаров. Fingerprint пересчитывается из сохранённых версий.
- Одна цифросодержащая лексема, которую текущий planner пометил ExactOnly без
  LimitReason, ищется только в OfferID/VendorCode.exact. Текст описания другого
  SKU не заменяет идентификатор. Fuzzy, parser и векторы не используются;
  остальные фильтры и priced-first сохраняются. Многословные запросы с версией
  продолжают обычный текстовый поиск без fuzzy чисел.
- Application повторяет проверку идентификатора, не доверяя адаптеру.

Сначала unit domain/application и HTTP doubles Elasticsearch; затем исправления,
реальный ES и cache-only tuning/development. Live-сервисы не останавливать для
fault injection: сбои воспроизводятся doubles, задержка/отмена — context.
Новые платные запросы и holdout в этой задаче не используются.

Query policy — query-v8; формат документа lexical-v7 и индекс не меняются.
Проверки не означают независимую оценку релевантности или готовый verifier.
Базовые права/совместимость расширений относятся к этапу 4.

## Результаты

Красные тесты воспроизвели поиск идентификатора в описаниях, принятие изменённых
catalog/family metadata и вызов векторов после отмены lexical. После исправления
прошли domain/application, HTTP doubles и реальный Elasticsearch. Проверены
также старые self-consistent, но несовместимые версии document/normalization
в Search и Resolve; timeout/failed shards не считаются пустой успешной выдачей.

Полный go test ./..., vet/build/gofmt и восемь Python-тестов прошли.
Cache-only tuning: 52/52 групп, 6/6 брендов, 2/2 отрицательных; development 7/16.
Порядок SKU всех 76 development-запросов совпал с предыдущей версией.
Это не независимая оценка: holdout не запускался, OpenAI не вызывался.

На полном каталоге: 540194/540220 возвращают соответствующие SKU; lrd01 —
487234, pt-xsp-pa4-add — 540194 по фактическому VendorCode без учёта регистра.
MISSING-SKU-999999 пуст; LRD2501 из URL также пуст, поскольку сохранённый артикул
RuDesktop — LRD01. URL-slug не считается доказательством артикула.
Во всех шести запросах vector=skipped_exact, parser=not_selected.
Активный индекс lexical-v7 остался
smart-search-eb46ed92f8e3672f04c5642468adab5546c5d4e45bca91e53eb607c705030a50.

Локальные отчёты: artifacts/search-recovery-tuning.json,
search-recovery-probes.json, search-recovery-identifiers.jsonl. В Git не включены.
Сервисы пользователя не останавливались, fault injection использовал doubles.
