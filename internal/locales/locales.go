package locales

type Translator struct {
	Lang string
}

var Translations = map[string]map[string]string{
	"en": {
		"dashboardTitle":      "Financial Monitor",
		"refresh":             "Refresh",
		"settings":            "Settings",
		"language":            "Language",
		"back":                "◀️ Back",
		"useStart":            "Send /start to open the dashboard.",
		"assetHeader":         "Asset Overview",
		"price":               "Current price",
		"change24h":           "24h change",
		"trend":               "Trend",
		"trendButton":         "Trend",
		"analytics":           "Analytics",
		"settingsHeader":      "Settings",
		"languageHeader":      "Language Settings",
		"selectLanguage":      "Choose interface language.",
		"languageCurrent":     "Current language:",
		"languageEnglish":     "English",
		"languageRussian":     "Русский",
		"languageChinese":     "中文",
		"usdEur":              "USD/EUR",
		"usdRub":              "USD/RUB",
		"eurRub":              "EUR/RUB",
		"noData":              "Data unavailable.",
		"updated":             "Updated:",
		"assetDetails":        "Asset details",
		"settingsInstruction": "Select a language to switch the interface.",
	},
	"ru": {
		"dashboardTitle":      "Финансовый монитор",
		"refresh":             "Обновить",
		"settings":            "Настройки",
		"language":            "Язык",
		"back":                "◀️ Назад",
		"useStart":            "Отправьте /start для открытия панели.",
		"assetHeader":         "Обзор актива",
		"price":               "Текущая цена",
		"change24h":           "Изменение 24ч",
		"trend":               "Тренд",
		"trendButton":         "Тренд",
		"analytics":           "Аналитика",
		"settingsHeader":      "Настройки",
		"languageHeader":      "Языковые настройки",
		"selectLanguage":      "Выберите язык интерфейса.",
		"languageCurrent":     "Текущий язык:",
		"languageEnglish":     "English",
		"languageRussian":     "Русский",
		"languageChinese":     "中文",
		"usdEur":              "USD/EUR",
		"usdRub":              "USD/RUB",
		"eurRub":              "EUR/RUB",
		"noData":              "Данные недоступны.",
		"updated":             "Обновлено:",
		"assetDetails":        "Детали актива",
		"settingsInstruction": "Выберите язык для переключения интерфейса.",
	},
	"zh": {
		"dashboardTitle":      "金融监控",
		"refresh":             "刷新",
		"settings":            "设置",
		"language":            "语言",
		"back":                "◀️ 返回",
		"useStart":            "发送 /start 以打开仪表盘。",
		"assetHeader":         "资产概览",
		"price":               "当前价格",
		"change24h":           "24小时变化",
		"trend":               "趋势",
		"trendButton":         "趋势",
		"analytics":           "分析",
		"settingsHeader":      "设置",
		"languageHeader":      "语言设置",
		"selectLanguage":      "选择界面语言。",
		"languageCurrent":     "当前语言：",
		"languageEnglish":     "English",
		"languageRussian":     "Русский",
		"languageChinese":     "中文",
		"usdEur":              "USD/EUR",
		"usdRub":              "USD/RUB",
		"eurRub":              "EUR/RUB",
		"noData":              "数据不可用。",
		"updated":             "更新时间：",
		"assetDetails":        "资产详情",
		"settingsInstruction": "请选择语言以切换界面。",
	},
}

func SupportedLanguages() []string {
	return []string{"en", "ru", "zh"}
}

func IsSupported(lang string) bool {
	_, ok := Translations[lang]
	return ok
}

func DefaultLanguage() string {
	return "en"
}

func Translate(lang, key string) string {
	if lang == "" {
		lang = DefaultLanguage()
	}
	if catalog, ok := Translations[lang]; ok {
		if text, ok := catalog[key]; ok {
			return text
		}
	}
	if catalog, ok := Translations[DefaultLanguage()]; ok {
		return catalog[key]
	}
	return ""
}

func NewTranslator(lang string) Translator {
	if !IsSupported(lang) {
		lang = DefaultLanguage()
	}
	return Translator{Lang: lang}
}

func (t Translator) T(key string) string {
	return Translate(t.Lang, key)
}

func (t Translator) With(lang string) Translator {
	if !IsSupported(lang) {
		lang = DefaultLanguage()
	}
	t.Lang = lang
	return t
}
