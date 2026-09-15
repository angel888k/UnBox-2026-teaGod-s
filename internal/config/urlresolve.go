package config

import (
	"net/url"
	"strings"
)

// ResolveRelativeURLs 把站点与直播源里写成相对路径的地址解析成绝对地址。
//
// 真实配置里常见 `./lib/drpy2.min.js`、`/spider.js` 这类写法，它们是相对于配置
// 自身地址的。不解析就会把相对路径当 URL 去请求，报
// `Get "./lib/drpy2.min.js": unsupported protocol scheme ""`。
//
// 只处理相对地址：已带协议（http/https/assets 等）、协议相对（//host/x）、
// 以及 `csp_` 这种蜘蛛类名保持原样。cfg.SourceURL 为空时不做任何改动。
func ResolveRelativeURLs(cfg *Config) {
	if cfg == nil || cfg.SourceURL == "" {
		return
	}
	base, err := url.Parse(cfg.SourceURL)
	if err != nil || base.Host == "" {
		return
	}
	for i := range cfg.Sites {
		cfg.Sites[i].API = resolveRelativeURL(base, cfg.Sites[i].API)
	}
	for i := range cfg.Lives {
		cfg.Lives[i].URL = resolveRelativeURL(base, cfg.Lives[i].URL)
	}
}

// resolveRelativeURL 把单个地址按 base 解析；不需要或无法解析时原样返回。
func resolveRelativeURL(base *url.URL, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || isSpiderClassName(trimmed) {
		return raw
	}
	ref, err := url.Parse(trimmed)
	if err != nil {
		return raw
	}
	// 已有协议（http://、assets://）或协议相对（//host/x）时不改写。
	if ref.Scheme != "" || ref.Host != "" {
		return raw
	}
	return base.ResolveReference(ref).String()
}

// isSpiderClassName 判断是否是蜘蛛类名（csp_xxx），它不是地址。
func isSpiderClassName(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "csp_")
}
