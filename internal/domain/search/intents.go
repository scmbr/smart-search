package search

import "regexp"

type ProductIntent struct{ FamilyID, Task, Text, RuleID, SourceURL string }

type productTaskRule struct {
	anchor, family, task, source string
	pattern                      *regexp.Regexp
}

var productTasks = []productTaskRule{
	{"astra linux", "astra-linux-special-edition", "operating_system", "https://docs.astralinux.ru/latest/szi/szi/", regexp.MustCompile(`(?:^|\s)(?:операционн(?:ая|ой|ую)\s+систем(?:а|ы|у)|операционк[ауи])`)},
	{"anydesk", "anydesk", "remote_access", "https://anydesk.com/en/features", regexp.MustCompile(`(?:^|\s)(?:для\s+)?(?:удаленк[аиу]|удаленн(?:ого|ый)\s+доступа?)`)},
	{"кибер бэкап", "cyber-backup", "backup", "https://docs.cyberprotect.ru/ru-RU/CyberBackup/17.0/user/backup-options.html", regexp.MustCompile(`(?:^|\s)резервн(?:ое|ого)\s+копировани[ея]`)},
	{"сканер вс", "scanner-vs", "vulnerability_scan", "https://npo-echelon.ru/scaner-vs-ins/", regexp.MustCompile(`(?:^|\s)(?:для\s+)?(?:проверк[аи]|поиск[аи])\s+уязвимост(?:ей|и)`)},
}

var familyIdentifier = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var negatedTask = regexp.MustCompile(`(?:^|\s)(?:не|без)\s+(?:для\s+)?(?:удаленк[аиу]|удаленн(?:ого|ый)\s+доступа?|проверк[аи]|поиск[аи]\s+уязвимост|резервн(?:ое|ого)|операционн(?:ая|ой|ую)|операционк[ауи])`)
