package config

type AppConfig struct {
	Debug         bool
	Silent        bool
	ScraperConfig ScraperConfig
}

func NewAppConfig(debug bool, silent bool, headless bool, proxyURL string, browserAddr string, dbPath string) AppConfig {
	scraperConfig := newScraperConfig(headless, proxyURL, browserAddr, dbPath)

	return AppConfig{
		Debug:         debug,
		Silent:        silent,
		ScraperConfig: scraperConfig,
	}
}

type ScraperConfig struct {
	Headless    bool
	ProxyURL    string
	BrowserAddr string
	DbPath      string
}

func newScraperConfig(headless bool, proxyURL string, browserAddr string, dbPath string) ScraperConfig {
	return ScraperConfig{
		Headless:    headless,
		ProxyURL:    proxyURL,
		BrowserAddr: browserAddr,
		DbPath:      dbPath,
	}
}
