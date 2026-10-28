package dashboard

import "testing"

func TestClassifyUA(t *testing.T) {
	cases := []struct {
		ua                       string
		browser, os, device, bot string
	}{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36", "Chrome", "Windows", "Desktop", ""},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36 Edg/120.0", "Edge", "Windows", "Desktop", ""},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.6 Mobile/15E148 Safari/604.1", "Safari", "iOS", "Mobile", ""},
		{"Mozilla/5.0 (Linux; Android 16; SM-S921U) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Mobile Safari/537.36", "Chrome", "Android", "Mobile", ""},
		{"Mozilla/5.0 (Linux; Android 13; SM-X700) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36", "Chrome", "Android", "Tablet", ""},
		{"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0", "Firefox", "Linux", "Desktop", ""},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "Safari", "macOS", "Desktop", ""},
		{"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "", "", "Bot", "Googlebot"},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", "", "", "Bot", "Bingbot"},
		{"curl/8.5.0", "", "", "Bot", "curl"},
		{"python-requests/2.31", "", "", "Bot", "python-requests"},
		{"-", "", "", "Bot", "No user agent"},
		{"", "", "", "Bot", "No user agent"},
		{"SomeUnknownCrawler/1.0", "", "", "Bot", "Other bot"},
		{"Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)", "", "", "Bot", "GPTBot"},
	}
	for _, c := range cases {
		got := classifyUA(c.ua)
		if got.Browser != c.browser || got.OS != c.os || got.Device != c.device || got.Bot != c.bot {
			t.Errorf("%q\n got  %+v\n want browser=%s os=%s device=%s bot=%s", c.ua, got, c.browser, c.os, c.device, c.bot)
		}
	}
}

func TestRefererHost(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.google.com/search?q=x":   "google.com",
		"http://t.co/abc":                     "t.co",
		"https://l.facebook.com:443/l.php":    "l.facebook.com",
		"-":                                   "",
		"":                                    "",
		"android-app://com.google.android.gm": "com.google.android.gm",
		"https://user:pw@Example.COM/path":    "example.com",
	} {
		if got := refererHost(in); got != want {
			t.Errorf("refererHost(%q) = %q, want %q", in, got, want)
		}
	}
}
