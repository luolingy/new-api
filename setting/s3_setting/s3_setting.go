package s3_setting

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// S3Setting stores the object storage configuration used for user uploads
// (currently profile avatars). Endpoint is the S3 API address, while
// PublicBaseURL is the address browsers should load objects from; both are kept
// separate because reverse proxies, CDNs and R2/MinIO custom domains usually
// differ from the signing endpoint.
type S3Setting struct {
	Enabled        bool   `json:"enabled"`
	Endpoint       string `json:"endpoint"`
	PublicBaseURL  string `json:"public_base_url"`
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	PathPrefix     string `json:"path_prefix"`
	ForcePathStyle bool   `json:"force_path_style"`
	AccessKey      string `json:"accessKey"`
	SecretKey      string `json:"secretKey"`
}

var defaultS3Setting = S3Setting{
	Region:         "us-east-1",
	PathPrefix:     "avatars",
	ForcePathStyle: true,
}

func init() {
	config.GlobalConfig.Register("s3", &defaultS3Setting)
}

// GetS3Setting returns the live configuration. The config manager replaces the
// struct contents in place, so callers must not retain the pointer across
// requests.
func GetS3Setting() *S3Setting {
	return &defaultS3Setting
}

// Configured reports whether every field required to sign a request is present.
func (s *S3Setting) Configured() bool {
	return strings.TrimSpace(s.Endpoint) != "" &&
		strings.TrimSpace(s.Bucket) != "" &&
		strings.TrimSpace(s.AccessKey) != "" &&
		strings.TrimSpace(s.SecretKey) != ""
}

// Ready reports whether avatar uploads can be accepted.
func (s *S3Setting) Ready() bool {
	return s.Enabled && s.Configured()
}

// NormalizedPrefix returns the object key prefix without surrounding slashes.
func (s *S3Setting) NormalizedPrefix() string {
	prefix := strings.Trim(strings.TrimSpace(s.PathPrefix), "/")
	if prefix == "" {
		return "avatars"
	}
	return prefix
}

var (
	bucketPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	regionPattern     = regexp.MustCompile(`^[a-z0-9-]+$`)
	pathPrefixPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// ValidateEndpoint accepts an empty value (uploads disabled) or an absolute
// http(s) URL without credentials, query string or fragment.
func ValidateEndpoint(value string) error {
	return validateHTTPBaseURL(value, "S3 Endpoint")
}

// ValidatePublicBaseURL accepts an empty value or an absolute http(s) URL. A
// path is allowed so that CDN prefixes keep working.
func ValidatePublicBaseURL(value string) error {
	return validateHTTPBaseURL(value, "S3 Public URL")
}

func validateHTTPBaseURL(value string, field string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s 不是合法的 URL", field)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s 必须以 http:// 或 https:// 开头", field)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s 缺少主机名", field)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s 不能包含用户名或密码", field)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s 不能包含查询参数或锚点", field)
	}
	return nil
}

// ValidateBucket accepts an empty value or a DNS-compatible bucket name.
func ValidateBucket(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !bucketPattern.MatchString(value) {
		return fmt.Errorf("Bucket 名称只能包含小写字母、数字、点和短横线，长度 3-63")
	}
	return nil
}

// ValidateRegion accepts an empty value or a simple region identifier.
func ValidateRegion(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || regionPattern.MatchString(value) {
		return nil
	}
	return fmt.Errorf("Region 只能包含小写字母、数字和短横线")
}

// ValidatePathPrefix accepts an empty value or a relative object prefix.
func ValidatePathPrefix(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !pathPrefixPattern.MatchString(value) {
		return fmt.Errorf("对象前缀只能包含字母、数字、点、下划线、斜杠和短横线")
	}
	if strings.Contains(value, "..") || strings.Contains(value, "//") {
		return fmt.Errorf("对象前缀不能包含 .. 或 //")
	}
	if strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return fmt.Errorf("对象前缀不能以 / 开头或结尾")
	}
	return nil
}
