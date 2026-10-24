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
	// BackgroundPosition is a CSS background-position keyword pair (e.g.
	// "center", "center top", "left center"). It lets a landscape image keep a
	// chosen region visible when a narrow viewport crops it.
	BackgroundPosition string `json:"background_position"`
	GlassCards         bool   `json:"glass_cards"`
}

var defaultSiteTheme = SiteTheme{
	BackgroundImage:    "",
	BackgroundPosition: DefaultBackgroundPosition,
	GlassCards:         true,
}

// DefaultBackgroundPosition centers the image on both axes.
const DefaultBackgroundPosition = "center"

// allowedBackgroundPositions is the fixed set of values the admin UI can pick.
// Restricting to keywords keeps the value safe inside an inline style and avoids
// arbitrary CSS injection through the option row.
var allowedBackgroundPositions = map[string]struct{}{
	"center":        {},
	"left top":      {},
	"center top":    {},
	"right top":     {},
	"left center":   {},
	"right center":  {},
	"left bottom":   {},
	"center bottom": {},
	"right bottom":  {},
}

var siteTheme = defaultSiteTheme

func init() {
	config.GlobalConfig.Register("theme", &siteTheme)
}

// GetSiteTheme returns a snapshot of the active site theme settings.
func GetSiteTheme() SiteTheme {
	return siteTheme
}

// NormalizedBackgroundPosition returns the stored position, falling back to the
// default when it is empty or not one of the accepted keywords.
func (s SiteTheme) NormalizedBackgroundPosition() string {
	position := strings.ToLower(strings.TrimSpace(s.BackgroundPosition))
	if _, ok := allowedBackgroundPositions[position]; ok {
		return position
	}
	return DefaultBackgroundPosition
}

// ValidateBackgroundPosition accepts an empty value (meaning center) or one of
// the allowed keyword pairs.
func ValidateBackgroundPosition(value string) error {
	position := strings.ToLower(strings.TrimSpace(value))
	if position == "" {
		return nil
	}
	if _, ok := allowedBackgroundPositions[position]; ok {
		return nil
	}
	return errors.New("背景图片焦点位置无效")
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
