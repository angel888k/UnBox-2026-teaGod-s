package config

import "testing"

func TestResolveRelativeURLs(t *testing.T) {
	cfg := &Config{
		SourceURL: "https://cfg.example.com/feed/api.json",
		Sites: []Site{
			{Key: "dot-slash", API: "./lib/drpy2.min.js"},
			{Key: "root-path", API: "/spider.js"},
			{Key: "bare", API: "spider.js"},
			{Key: "parent", API: "../common/a.js"},
			{Key: "absolute", API: "https://cdn.example.com/down.php/x.js"},
			{Key: "protocol-relative", API: "//cdn.example.com/x.js"},
			{Key: "jar-class", API: "csp_WexAppV7Guard"},
			{Key: "asset", API: "assets://js/lib/cheerio.min.js"},
			{Key: "empty", API: ""},
		},
		Lives: LiveList{{Name: "本地", URL: "./live/iptv.m3u"}},
	}

	ResolveRelativeURLs(cfg)

	want := map[string]string{
		"dot-slash":         "https://cfg.example.com/feed/lib/drpy2.min.js",
		"root-path":         "https://cfg.example.com/spider.js",
		"bare":              "https://cfg.example.com/feed/spider.js",
		"parent":            "https://cfg.example.com/common/a.js",
		"absolute":          "https://cdn.example.com/down.php/x.js",
		"protocol-relative": "//cdn.example.com/x.js",
		"jar-class":         "csp_WexAppV7Guard",
		"asset":             "assets://js/lib/cheerio.min.js",
		"empty":             "",
	}
	for _, site := range cfg.Sites {
		if got := site.API; got != want[site.Key] {
			t.Errorf("站点 %s: API = %q，期望 %q", site.Key, got, want[site.Key])
		}
	}
	if got := cfg.Lives[0].URL; got != "https://cfg.example.com/feed/live/iptv.m3u" {
		t.Errorf("直播 URL = %q", got)
	}
}

func TestResolveRelativeURLsWithoutSourceURL(t *testing.T) {
	cfg := &Config{Sites: []Site{{Key: "a", API: "./x.js"}}}
	ResolveRelativeURLs(cfg)
	if cfg.Sites[0].API != "./x.js" {
		t.Fatalf("无来源地址时不应改写: %q", cfg.Sites[0].API)
	}
	ResolveRelativeURLs(nil) // 不应 panic
}
