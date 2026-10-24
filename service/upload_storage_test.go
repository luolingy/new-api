package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/setting/s3_setting"
	"github.com/QuantumNous/new-api/setting/storage_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateUploadsTempDir points the uploads directory at a fresh temp dir for the
// duration of one test.
func isolateUploadsTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("UPLOADS_DIR", dir)
	return dir
}

func TestSaveAndResolveLocalUpload(t *testing.T) {
	dir := isolateUploadsTempDir(t)
	pngData := encodeTestImage(t, "png", 16, 16)

	publicPath, err := SaveLocalUpload("avatars", "png", pngData)
	require.NoError(t, err)
	assert.Regexp(t, `^/uploads/avatars/[0-9]+-[0-9a-f]+\.png$`, publicPath)

	diskPath, contentType, ok := LocalUploadFile(publicPath)
	require.True(t, ok)
	assert.Equal(t, "image/png", contentType)
	assert.True(t, filepath.HasPrefix(diskPath, filepath.Clean(dir)))

	written, err := os.ReadFile(diskPath)
	require.NoError(t, err)
	assert.Equal(t, pngData, written)

	// Deleting removes the file; deleting again is a no-op.
	require.NoError(t, DeleteLocalUpload(publicPath))
	_, err = os.Stat(diskPath)
	assert.True(t, os.IsNotExist(err))
	require.NoError(t, DeleteLocalUpload(publicPath))
}

func TestLocalUploadRejectsTraversalAndUnknownExtensions(t *testing.T) {
	isolateUploadsTempDir(t)
	for _, publicPath := range []string{
		"/uploads/../../etc/passwd",
		"/uploads/../secret.png",
		"/uploads/a/../../b.png",
		"/uploads/",
		"/uploads",
		"https://evil.example.com/uploads/a.png",
		"/other/a.png",
	} {
		_, _, ok := LocalUploadFile(publicPath)
		assert.Falsef(t, ok, "expected %q to be rejected", publicPath)
		assert.Falsef(t, IsLocalUploadPath(publicPath), "expected %q not local", publicPath)
	}

	// An extension outside the allowlist resolves to nothing even inside the dir.
	require.NoError(t, os.MkdirAll(LocalUploadsDir(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(LocalUploadsDir(), "note.txt"), []byte("x"), 0o644))
	_, _, ok := LocalUploadFile("/uploads/note.txt")
	assert.False(t, ok, "unknown extensions must not be served")

	// Save refuses an unknown extension too.
	_, err := SaveLocalUpload("avatars", "txt", []byte("x"))
	assert.Error(t, err)

	// A duplicate slash (e.g. from a "//" URL) still resolves inside the tree.
	require.NoError(t, os.WriteFile(filepath.Join(LocalUploadsDir(), "ok.png"), []byte("x"), 0o644))
	_, _, ok = LocalUploadFile("/uploads//ok.png")
	assert.True(t, ok, "duplicate slashes must not break resolution")
}

func TestValidateUploadedImage(t *testing.T) {
	contentType, ext, err := ValidateUploadedImage(encodeTestImage(t, "png", 64, 32), 8192)
	require.NoError(t, err)
	assert.Equal(t, "image/png", contentType)
	assert.Equal(t, "png", ext)

	contentType, ext, err = ValidateUploadedImage(encodeTestImage(t, "jpeg", 64, 32), 8192)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", contentType)
	assert.Equal(t, "jpg", ext)

	// A dimension beyond the caller's budget is rejected.
	_, _, err = ValidateUploadedImage(encodeTestImage(t, "png", 200, 10), 100)
	assert.Error(t, err)

	_, _, err = ValidateUploadedImage([]byte("definitely not an image"), 8192)
	assert.Error(t, err)

	// GIF is a valid image but not an accepted upload format.
	gifHeader := []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;")
	_, _, err = ValidateUploadedImage(gifHeader, 8192)
	assert.Error(t, err)
}

func TestResolveUploadStorageAndDispatch(t *testing.T) {
	isolateUploadsTempDir(t)
	storage := storage_setting.GetStorageSetting()
	s3 := s3_setting.GetS3Setting()
	previousMode, previousDir := storage.Mode, storage.LocalDir
	previousEnabled := s3.Enabled
	t.Cleanup(func() {
		storage.Mode, storage.LocalDir = previousMode, previousDir
		s3.Enabled = previousEnabled
	})

	// Auto falls back to local when S3 is not ready.
	storage.Mode = storage_setting.ModeAuto
	s3.Enabled = false
	assert.Equal(t, UploadBackendLocal, ResolveUploadStorage().Backend)

	// Explicit local wins even if S3 were ready.
	storage.Mode = storage_setting.ModeLocal
	assert.Equal(t, UploadBackendLocal, ResolveUploadStorage().Backend)

	// Local dispatch writes the file and returns a local URL.
	pngData := encodeTestImage(t, "png", 16, 16)
	imageURL, err := StoreUploadedImage(context.Background(), "theme", NewThemeObjectKey("png"), "png", "image/png", pngData)
	require.NoError(t, err)
	assert.True(t, IsLocalUploadPath(imageURL))
	assert.Regexp(t, `^/uploads/theme/[0-9]+-[0-9a-f]+\.png$`, imageURL)

	// Deleting through the dispatch removes the local file.
	DeleteUploadedImageByURL(context.Background(), imageURL)
	_, _, ok := LocalUploadFile(imageURL)
	require.True(t, ok, "the path itself stays well-formed")
	_, statErr := os.Stat(filepath.Join(LocalUploadsDir(), "theme"))
	// The theme directory remains but no longer holds the deleted file.
	if statErr == nil {
		entries, _ := os.ReadDir(filepath.Join(LocalUploadsDir(), "theme"))
		assert.Empty(t, entries)
	}

	// Forcing S3 without configuration surfaces a clear error instead of writing locally.
	storage.Mode = storage_setting.ModeS3
	_, err = StoreUploadedImage(context.Background(), "theme", NewThemeObjectKey("png"), "png", "image/png", pngData)
	assert.Error(t, err)
}
