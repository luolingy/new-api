package storage_setting

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// Upload backends. Auto prefers S3 when it is enabled and fully configured and
// otherwise keeps files on the local disk, so a deployment without object
// storage can still accept uploads.
const (
	ModeAuto  = "auto"
	ModeLocal = "local"
	ModeS3    = "s3"
)

// DefaultLocalDir keeps uploaded images beside the other runtime data. The
// UPLOADS_DIR environment variable overrides it at request time.
const DefaultLocalDir = "data/uploads"

// StorageSetting selects where uploaded images (profile avatars and the site
// theme background) are written.
type StorageSetting struct {
	Mode     string `json:"mode"`
	LocalDir string `json:"local_dir"`
}

var defaultStorageSetting = StorageSetting{
	Mode:     ModeAuto,
	LocalDir: DefaultLocalDir,
}

func init() {
	config.GlobalConfig.Register("storage", &defaultStorageSetting)
}

// GetStorageSetting returns the live configuration. The config manager replaces
// the struct contents in place, so callers must not retain the pointer across
// requests.
func GetStorageSetting() *StorageSetting {
	return &defaultStorageSetting
}

// NormalizedMode collapses an empty or unrecognized mode to auto.
func (s *StorageSetting) NormalizedMode() string {
	switch strings.ToLower(strings.TrimSpace(s.Mode)) {
	case ModeLocal:
		return ModeLocal
	case ModeS3:
		return ModeS3
	default:
		return ModeAuto
	}
}

// NormalizedLocalDir returns the configured upload directory, or the default
// when the option is empty.
func (s *StorageSetting) NormalizedLocalDir() string {
	dir := strings.TrimSpace(s.LocalDir)
	if dir == "" {
		return DefaultLocalDir
	}
	return filepath.Clean(dir)
}

// ValidateMode accepts an empty value (auto) or one of the known backends.
func ValidateMode(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ModeAuto, ModeLocal, ModeS3:
		return nil
	}
	return fmt.Errorf("上传存储方式只能是 auto、local 或 s3")
}

// ValidateLocalDir accepts an empty value or a directory that does not escape
// its parent through ".." segments.
func ValidateLocalDir(value string) error {
	dir := strings.TrimSpace(value)
	if dir == "" {
		return nil
	}
	if strings.ContainsRune(dir, 0) {
		return fmt.Errorf("上传目录包含无效字符")
	}
	for segment := range strings.SplitSeq(filepath.ToSlash(dir), "/") {
		if segment == ".." {
			return fmt.Errorf("上传目录不能包含 ..")
		}
	}
	return nil
}
