package service

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/storage_setting"
)

// LocalUploadsPublicPrefix is the URL prefix that serves locally stored
// uploads. It is a fixed constant so saved image URLs stay valid across
// restarts and configuration changes.
const LocalUploadsPublicPrefix = "/uploads"

// localUploadExtensions maps the file extensions this site is willing to serve
// to their content types. Anything else is reported as missing, so a stray file
// placed in the uploads directory can never be published.
var localUploadExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// LocalUploadsDir returns the directory that stores local uploads.
// UPLOADS_DIR overrides the configured option so containers can mount it.
func LocalUploadsDir() string {
	if dir := strings.TrimSpace(os.Getenv("UPLOADS_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	return storage_setting.GetStorageSetting().NormalizedLocalDir()
}

// SaveLocalUpload writes one image below <dir>/<group>/ and returns the public
// path that the uploads route serves.
func SaveLocalUpload(group string, extension string, data []byte) (string, error) {
	ext := normalizeUploadExtension(extension)
	if _, ok := localUploadExtensions[ext]; !ok {
		return "", fmt.Errorf("不支持保存 %s 格式的图片", extension)
	}
	group = strings.Trim(strings.TrimSpace(group), "/")
	if group == "" || strings.Contains(group, "..") {
		return "", errors.New("上传分类无效")
	}
	if len(data) == 0 {
		return "", errors.New("上传文件为空")
	}
	dir := filepath.Join(LocalUploadsDir(), filepath.FromSlash(group))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建上传目录: %w", err)
	}
	name := fmt.Sprintf("%d-%s%s", time.Now().Unix(), randomHex(6), ext)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", fmt.Errorf("无法写入上传文件: %w", err)
	}
	return path.Join(LocalUploadsPublicPrefix, group, name), nil
}

// IsLocalUploadPath reports whether publicPath points at this site's uploads
// directory.
func IsLocalUploadPath(publicPath string) bool {
	_, ok := localUploadRelativePath(publicPath)
	return ok
}

// DeleteLocalUpload removes a file previously written by SaveLocalUpload. Paths
// outside this site's uploads directory and missing files are not errors.
func DeleteLocalUpload(publicPath string) error {
	rel, ok := localUploadRelativePath(publicPath)
	if !ok {
		return nil
	}
	err := os.Remove(filepath.Join(LocalUploadsDir(), filepath.FromSlash(rel)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LocalUploadFile resolves a public uploads path to the file on disk and its
// content type. ok is false when the path escapes the uploads directory or uses
// an extension this site does not serve.
func LocalUploadFile(publicPath string) (diskPath string, contentType string, ok bool) {
	rel, ok := localUploadRelativePath(publicPath)
	if !ok {
		return "", "", false
	}
	contentType, ok = localUploadExtensions[strings.ToLower(filepath.Ext(rel))]
	if !ok {
		return "", "", false
	}
	return filepath.Join(LocalUploadsDir(), filepath.FromSlash(rel)), contentType, true
}

// normalizeUploadExtension lowercases a file extension and guarantees a single
// leading dot, so "png", ".PNG" and ".png" all map to the same allowlist key.
func normalizeUploadExtension(extension string) string {
	ext := strings.ToLower(strings.TrimSpace(extension))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}

// localUploadRelativePath strips the public prefix and rejects any path that
// could escape the uploads directory once joined with it.
func localUploadRelativePath(publicPath string) (string, bool) {
	trimmed := strings.TrimSpace(publicPath)
	prefix := LocalUploadsPublicPrefix + "/"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	rel := path.Clean(strings.TrimPrefix(trimmed, prefix))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return "", false
	}
	return rel, true
}
