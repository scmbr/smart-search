package search

import "testing"

func TestProductRoleEvidence(t *testing.T) {
	for _, tc := range []struct {
		doc  Document
		role string
	}{
		{Document{Name: "Дополнительные компоненты продукта, полная лицензия"}, "addon"},
		{Document{Name: "КриптоПро CSP TLS, лицензия на расширение права использования"}, "addon"},
		{Document{Name: "Продукт", Description: "Тип лицензии: расширение лицензии • Минимальная покупка: 1"}, "addon"},
		{Document{Name: "Продукт (медиа-комплект), цена за 1 дистрибутив"}, "media"},
		{Document{Name: "Продукт", Description: "Тип лицензии: дистрибутив • Платформа: Linux"}, "media"},
		{Document{Name: "Продукт (стандартная техническая поддержка), для клиентской лицензии"}, "support"},
		{Document{Name: "Продукт (лицензия), One Time Setup Fee"}, "service"},
		{Document{ProductName: "Сервис непрерывного обследования"}, "service"},
		{Document{ProductName: "Сервис резервного копирования"}, ""},
		{Document{Name: "Поставщик для интеграции с провайдером публичного облака"}, "addon"},
		{Document{Name: "Chart SDK для разработчиков"}, "library"},
		{Document{Name: "Библиотека графиков для .NET"}, "library"},
		{Document{Name: "Программа управления библиотекой компании"}, ""},
		{Document{CategoryNames: []string{"Программы для библиотек", "Управление бизнесом"}}, ""},
		{Document{CategoryNames: []string{"Библиотеки и компоненты для разработчиков", "Разработка"}}, "library"},
		{Document{Name: "Продукт", Description: "Поддерживает библиотеки и расширения, включает техническую поддержку"}, ""},
		{Document{Name: "Продукт, полная лицензия + 2 года техподдержки"}, ""},
		{Document{Name: "Продукт, лицензия + медиа-комплект"}, ""},
		{Document{Name: "SDK Kit", ProductType: "hardware"}, ""},
		{Document{Name: "GoDiagram", GroupID: "208224", URL: "https://store.softline.ru/northwoodssoftware/sku/"}, "library"},
		{Document{Name: "GoDiagram", GroupID: "208224", URL: "https://store.softline.ru/other/sku/"}, ""},
		{Document{Name: "Aspose.Diagram", GroupID: "174704", URL: "https://store.softline.ru/aspose-pty-ltd/sku/"}, "library"},
	} {
		evidence := ClassifyProductRole(tc.doc)
		if evidence.Role != tc.role {
			t.Fatalf("%+v: %+v want %s", tc.doc, evidence, tc.role)
		}
		if evidence.Role != "" && (evidence.RuleID == "" || evidence.Source == "" || evidence.Text == "") {
			t.Fatal("missing evidence", evidence)
		}
	}
}

func TestProductRoleRequests(t *testing.T) {
	for _, tc := range []struct {
		query, role, productType string
		allowAddon               bool
	}{
		{"средство криптографической защиты", "", "", false},
		{"библиотека диаграмм SDK", "library", "", false},
		{"дополнительный компонент продукта", "addon", "", true},
		{"медиа-комплект продукта", "media", "", false},
		{"услуга установки продукта", "service", "", false},
		{"приложение для рисования, не библиотека", "", "", false},
		{"программа управления библиотекой компании", "", "software", false},
		{"программа для библиотеки", "", "software", false},
		{"AB-1234", "", "", true},
		{"SDK-1234", "", "", true},
		{"офисные приложения для документов", "", "software", false},
		{"защищенная российская операционная система", "", "software", false},
		{"сервер для программного обеспечения", "", "", false},
	} {
		r, err := Prepare(Request{Query: tc.query, Limit: 4})
		if err != nil || r.RequestedProductRole != tc.role || r.ProductType != tc.productType {
			t.Fatalf("%s: %+v %v", tc.query, r, err)
		}
		if ProductRoleAllowed(r, "addon") != tc.allowAddon {
			t.Fatalf("%s role policy", tc.query)
		}
		if tc.role != "" && ProductRoleAllowed(r, "") {
			t.Fatal("unknown satisfied strict role")
		}
	}
	if _, err := Prepare(Request{Query: "библиотека и дополнение", Limit: 4}); err == nil {
		t.Fatal("ambiguous role silently selected")
	}
}

func TestNamedReviewedLibraryIsStillSearchable(t *testing.T) {
	for _, query := range []string{"GoDiagram", "Northwoods Software", "Aspose.Diagram", "Aspose"} {
		r, err := Prepare(Request{Query: query, Limit: 4})
		if err != nil || !ProductRoleAllowed(r, "library") || !ProductRoleAllowed(r, "") || ProductRoleAllowed(r, "addon") {
			t.Fatalf("%s: %+v %v", query, r, err)
		}
	}
}
