package theme_setting

import (
	"errors"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// maxBackgroundImageURLLength bounds the option value so a malformed paste
// cannot bloat the option table or the /api/status payload.
const maxBackgroundImageURLLength = 2048

// SiteTheme is the optional site-wide frontend theme. The background image is
// the activation switch: an empty URL leaves the default dashboard untouched,
// while a configured URL enables the glass card treatment on top of it.
type SiteTheme struct {
	BackgroundImage string `json:"background_image"`
	GlassCards      bool   `json:"glass_cards"`
}

var defaultSiteTheme = SiteTheme{
	BackgroundImage: "",
	GlassCards:      true,
}

var siteTheme = defaultSiteTheme

func init() {
	config.GlobalConfig.Register("theme", &siteTheme)
}

// GetSiteTheme returns a snapshot of the active site theme settings.
func GetSiteTheme() SiteTheme {
	return siteTheme
}

// ValidateBackgroundImageURL accepts only absolute http(s) URLs or a
// root-relative path. The value ends up inside a CSS url() token, so quotes,
// parentheses, backslashes, angle brackets, and whitespace are rejected
// outright instead of being escaped downstream.
func ValidateBackgroundImageURL(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > maxBackgroundImageURLLength {
		return errors.New("背景图片 URL 过长")
	}
	if strings.ContainsAny(trimmed, "\"'()\\ \t\r\n<>") {
		return errors.New("背景图片 URL 包含不支持的字符")
	}
	if strings.HasPrefix(trimmed, "//") {
		return errors.New("背景图片 URL 不能使用协议相对地址")
	}
	if strings.HasPrefix(trimmed, "/") {
		return nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return errors.New("背景图片 URL 格式不正确")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("背景图片 URL 必须使用 http 或 https")
	}
	if parsed.Host == "" {
		return errors.New("背景图片 URL 缺少主机名")
	}
	return nil
}
