"""Render the 100 draft judgments for human review; never execute search."""
import argparse
import json
from pathlib import Path

TAGS = {"exact":"название", "typo":"опечатка", "transliteration":"транслитерация",
        "slang":"сленг", "brand":"бренд", "task_intent":"задача",
        "license_constraint":"лицензия", "version":"версия", "mixed_language":"смешанный язык",
        "punctuation":"пунктуация", "ambiguous":"неоднозначность", "out_of_catalog":"нет в каталоге",
        "exact_sku":"точный SKU"}
KEYS = {"exclude_offer_kinds":"исключить", "offer_kind":"тип лицензии", "edition":"редакция",
        "version":"версия", "product_type":"тип продукта", "min_users":"пользователей ≥",
        "min_devices":"устройств ≥", "term_months":"месяцев", "deployment":"назначение",
        "min_participants":"участников ≥", "min_ip_addresses":"IP-адресов ≥", "term":"срок",
        "availability_policy":"наличие"}
VALUES = {"renewal":"продление", "upgrade":"обновление", "support":"поддержка",
          "new_license":"новая лицензия", "software":"ПО", "hardware":"оборудование",
          "server":"сервер", "perpetual":"бессрочно", "available_only":"только доступные"}
BRANDS = {"astralinux":"Астра", "microsoft":"Microsoft", "kaspersky":"Kaspersky",
          "kripto-pro":"КриптоПро", "anydesk":"AnyDesk", "cyberprotect":"Киберпротект",
          "npo-echelon":"НПО Эшелон", "webinar":"МТС Линк", "positive-technologies":"Positive Technologies",
          "adobe":"Adobe", "trueconf":"TrueConf", "drweb":"Dr.Web", "entensys":"UserGate"}


def cell(value):
    return str(value).replace("|", "\\|").replace("\n", " ")


def render():
    lines = ["# Проверка 100 поисковых кейсов", "",
             "**60 tuning + 40 holdout. Все ответы — предварительная разметка, не результаты поиска.**",
             "Поиск на holdout не запускался. Исторические regression и 10 исходных примеров",
             "сохранены отдельно и не входят в эти 100. Все варианты бренда остаются на одной стороне.", "",
             "Для проверки прочитайте запрос и ожидаемые группы. Ссылки ведут на примеры SKU",
             "из предоставленного снимка; это не обязательно подходящая лицензия. Не нужно",
             "проверять все предложения группы. Замечания пишите по ID: другая цель, недостающая",
             "группа, неоднозначность или неверное ограничение. Не используйте выдачу поиска",
             "для подгонки ожидаемых ответов holdout.", "",
             "Для широкого бренда приведены примеры, а не исчерпывающий список правильных товаров.",
             "Другой продукт того же бренда не становится ошибкой лишь из-за отсутствия в списке.",
             "Четыре семейства допускают только подходящие товары и проверенные допродажи;",
             "таблица не доказывает совместимость и не требует случайного дополнения выдачи.", "",
             "Источники истины: [tuning](../testdata/evaluation/scenarios.json) и",
             "[holdout](../testdata/evaluation/holdout-v2.json). Статус всех строк: **требует проверки**.",
             "Таблица генерируется: `python scripts/render-evaluation.py`; проверка: `--check`."]
    for filename, title in [("scenarios.json", "Tuning — 60 кейсов"), ("holdout-v2.json", "Holdout — 40 кейсов")]:
        dataset = json.loads((Path("testdata/evaluation") / filename).read_text(encoding="utf-8"))
        lines += ["", "## " + title, "", "| ID | Запрос | Тип | Ожидаемые группы | Ограничения и замечания |", "| --- | --- | --- | --- | --- |"]
        for case in dataset["cases"]:
            scope = case["target_scope"]
            targets = []
            for group in case["group_ids"]:
                evidence = dataset["catalog_evidence"][group]
                targets.append(f'[{cell(evidence["group_name"])} ({group})]({evidence["example_url"]})')
            expected = "; ".join(targets) if targets else "Пустой ответ, без случайных товаров"
            if scope == "brand":
                expected = "Бренд " + BRANDS[case["brand_id"]] + "; примеры: " + expected
            constraints = []
            for key, value in case["constraints"].items():
                translated = ", ".join(VALUES.get(v, str(v)) for v in value) if isinstance(value, list) else VALUES.get(str(value), str(value))
                constraints.append(KEYS.get(key, key) + ": " + translated)
            note = "; ".join(constraints)
            if case["note_ru"]:
                note += (". " if note else "") + case["note_ru"]
            tags = ", ".join(TAGS[tag] for tag in case["tags"])
            lines.append("| " + " | ".join([cell(case["id"]), cell(case["query"]), tags, expected, cell(note)]) + " |")
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    path = Path("docs/evaluation-cases.md")
    expected = render()
    if args.check:
        if not path.exists() or path.read_text(encoding="utf-8") != expected:
            raise SystemExit("FAIL: regenerate docs/evaluation-cases.md with scripts/render-evaluation.py")
        print("PASS: review table matches the 100 draft judgments")
    else:
        path.write_text(expected, encoding="utf-8")


if __name__ == "__main__":
    main()
