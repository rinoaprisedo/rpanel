package dashboard

import "strings"

// Client is what a User-Agent string says about the visitor.
type Client struct {
	Browser string // "Chrome", "Safari", ... ; "" for bots
	OS      string // "Windows", "Android", ...
	Device  string // "Desktop" | "Mobile" | "Tablet" | "Bot"
	Bot     string // a recognised crawler name, "Other bot" or "Script"; "" for people
}

type named struct{ marker, name string }

// Order matters: the first marker found wins.
var knownBots = []named{
	{"googlebot", "Googlebot"}, {"google-inspectiontool", "Googlebot"}, {"adsbot-google", "Google Ads"}, {"apis-google", "Google"},
	{"bingbot", "Bingbot"}, {"bingpreview", "Bingbot"}, {"msnbot", "Bingbot"},
	{"yandex", "YandexBot"}, {"baiduspider", "Baidu"}, {"duckduckbot", "DuckDuckBot"}, {"slurp", "Yahoo"},
	{"applebot", "Applebot"}, {"ahrefsbot", "AhrefsBot"}, {"semrushbot", "SemrushBot"}, {"mj12bot", "MJ12bot"}, {"dotbot", "DotBot"},
	{"petalbot", "PetalBot"}, {"bytespider", "Bytespider"},
	{"gptbot", "GPTBot"}, {"chatgpt-user", "ChatGPT"}, {"oai-searchbot", "OpenAI"}, {"claudebot", "ClaudeBot"}, {"anthropic", "Anthropic"}, {"perplexitybot", "Perplexity"},
	{"facebookexternalhit", "Facebook"}, {"facebot", "Facebook"}, {"twitterbot", "Twitter"}, {"linkedinbot", "LinkedIn"}, {"whatsapp", "WhatsApp"}, {"telegrambot", "Telegram"}, {"slackbot", "Slack"}, {"discordbot", "Discord"},
	{"uptimerobot", "UptimeRobot"}, {"pingdom", "Pingdom"}, {"statuscake", "StatusCake"}, {"zabbix", "Zabbix"},
	{"curl/", "curl"}, {"wget", "Wget"}, {"python-requests", "python-requests"}, {"python-urllib", "python-urllib"}, {"go-http-client", "Go HTTP client"},
	{"okhttp", "OkHttp"}, {"java/", "Java"}, {"libwww", "libwww"}, {"scrapy", "Scrapy"}, {"httpclient", "HTTP client"}, {"axios", "axios"}, {"node-fetch", "node-fetch"},
	{"headlesschrome", "Headless Chrome"}, {"phantomjs", "PhantomJS"},
}

var genericBotMarkers = []string{"bot", "spider", "crawl", "slurp", "scan", "monitor", "check", "probe", "fetch", "scrap"}

func detectBot(ua string) string {
	u := strings.ToLower(ua)
	if u == "" || u == "-" {
		return "No user agent"
	}
	for _, k := range knownBots {
		if strings.Contains(u, k.marker) {
			return k.name
		}
	}
	for _, m := range genericBotMarkers {
		if strings.Contains(u, m) {
			return "Other bot"
		}
	}
	return ""
}

func detectOS(u string) string {
	switch {
	case strings.Contains(u, "windows phone"):
		return "Windows Phone"
	case strings.Contains(u, "windows"):
		return "Windows"
	case strings.Contains(u, "iphone") || strings.Contains(u, "ipad") || strings.Contains(u, "ipod"):
		return "iOS"
	case strings.Contains(u, "android"):
		return "Android"
	case strings.Contains(u, "cros"):
		return "ChromeOS"
	case strings.Contains(u, "mac os x") || strings.Contains(u, "macintosh"):
		return "macOS"
	case strings.Contains(u, "ubuntu") || strings.Contains(u, "debian") || strings.Contains(u, "fedora") || strings.Contains(u, "linux") || strings.Contains(u, "x11"):
		return "Linux"
	}
	return "Other"
}

func detectBrowser(u string) string {
	switch {
	case strings.Contains(u, "edg/") || strings.Contains(u, "edge/") || strings.Contains(u, "edga/") || strings.Contains(u, "edgios/"):
		return "Edge"
	case strings.Contains(u, "opr/") || strings.Contains(u, "opera"):
		return "Opera"
	case strings.Contains(u, "samsungbrowser"):
		return "Samsung Internet"
	case strings.Contains(u, "firefox/") || strings.Contains(u, "fxios"):
		return "Firefox"
	case strings.Contains(u, "msie ") || strings.Contains(u, "trident/"):
		return "Internet Explorer"
	case strings.Contains(u, "crios") || strings.Contains(u, "chrome/") || strings.Contains(u, "chromium"):
		return "Chrome"
	case strings.Contains(u, "safari/") || strings.Contains(u, "applewebkit"):
		return "Safari"
	}
	return "Other"
}

// classifyUA labels a User-Agent. Crawlers and scripts are recognised first so they never count as people.
func classifyUA(ua string) Client {
	if b := detectBot(ua); b != "" {
		return Client{Device: "Bot", Bot: b}
	}
	u := strings.ToLower(ua)
	c := Client{Browser: detectBrowser(u), OS: detectOS(u), Device: "Desktop"}
	switch {
	case strings.Contains(u, "ipad") || strings.Contains(u, "tablet") || (strings.Contains(u, "android") && !strings.Contains(u, "mobile")):
		c.Device = "Tablet"
	case strings.Contains(u, "mobile") || strings.Contains(u, "iphone") || strings.Contains(u, "ipod") || strings.Contains(u, "android"):
		c.Device = "Mobile"
	}
	return c
}

// refererHost reduces a Referer header to its host ("" for none/invalid).
func refererHost(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "-" {
		return ""
	}
	if i := strings.Index(ref, "://"); i >= 0 {
		ref = ref[i+3:]
	}
	if i := strings.IndexAny(ref, "/?#"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, "@"); i >= 0 {
		ref = ref[i+1:]
	}
	if i := strings.Index(ref, ":"); i >= 0 {
		ref = ref[:i]
	}
	ref = strings.ToLower(strings.TrimPrefix(strings.ToLower(ref), "www."))
	if ref == "" || strings.ContainsAny(ref, " \t\"") {
		return ""
	}
	return ref
}
