# CI и автономные PR

Обязательный check для main: **quality-gate** из workflows/quality.yml.

- repository-quality: UTF-8, локальные Markdown-ссылки на отслеживаемые файлы,
  JSON, имена PR/веток и actionlint v1.7.12.
- go-quality: Linux/Windows, gofmt, go vet, go build и go test. Включается,
  когда go.mod присутствует в коммите. Версия Go берётся из go.mod с актуальным patch.
- postgres-quality: PostgreSQL 17.12, реальные миграции, атомарность порций,
  возобновление, блокировка writer и переключение active; требуется quality-gate.
  Тот же job поднимает Elasticsearch 9.5.7 и проверяет реальный индекс/поиск:
  фильтры, collapse, версии, exact SKU и priced/unpriced fallback.
- frontend-quality: включается при frontend/package.json; требует .nvmrc,
  npm lockfile и scripts lint, lint:arch, typecheck, test:ci, build.
- quality-gate: всегда запускается и отклоняет failure/cancelled/неожиданный skip.
  Отсутствующее приложение отмечается skipped, не выдаётся за проверенное.

CI не использует ключи моделей и не обращается к платным API. Actions закреплены
SHA; права workflow — contents:read. Скрипт проверяет git index/commit, а не
неотслеживаемые файлы. Локальная команда: `python .github/scripts/check_repository.py`.
Go-файлы, go.mod/go.sum и SQL-миграции закреплены как LF в .gitattributes,
включая Windows checkout: иначе меняются gofmt и checksum встроенной миграции.

Настройки GitHub: auto-merge и удаление слитых веток включены, squash разрешён;
main защищена обязательным quality-gate, актуальностью ветки, PR и линейной историей.
Требование человеческого approve не включается: одиночный автор не может одобрить
собственный PR. Защита действует и для admin; force-push/удаление main запрещены.
Если применение настройки не удалось, ориентироваться на проверенный ответ API,
а не считать этот документ подтверждением фактического состояния GitHub.

Первый PR настройки содержал только инструкции и CI; Go-job в нём был пропущен.
После публикации reader/CLI, go.mod и fixtures Go-проверки обязательны на обеих ОС.
