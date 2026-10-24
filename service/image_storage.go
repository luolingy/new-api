package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/setting/s3_setting"
	"github.com/QuantumNous/new-api/setting/storage_setting"
)

// Upload backend identifiers reported by ResolveUploadStorage.
const (
	UploadBackendLocal = "local"
	UploadBackendS3    = "s3"
)

// UploadStorage is the backend that will receive the next uploaded image.
type UploadStorage struct {
	Backend string
}

// ResolveUploadStorage picks the upload backend from the configured mode,
// falling back to the local disk whenever S3 is not ready. This keeps a fresh
// install able to accept uploads without any object storage.
func ResolveUploadStorage() UploadStorage {
	switch storage_setting.GetStorageSetting().NormalizedMode() {
	case storage_setting.ModeLocal:
		return UploadStorage{Backend: UploadBackendLocal}
	case storage_setting.ModeS3:
		return UploadStorage{Backend: UploadBackendS3}
	default:
		if s3_setting.GetS3Setting().Ready() {
			return UploadStorage{Backend: UploadBackendS3}
		}
		return UploadStorage{Backend: UploadBackendLocal}
	}
}

// StoreUploadedImage writes one uploaded image through the active backend and
// returns the URL browsers should load. group is the local sub-directory and
// objectKey the key used by object storage.
func StoreUploadedImage(ctx context.Context, group, objectKey, extension, contentType string, data []byte) (string, error) {
	if ResolveUploadStorage().Backend == UploadBackendS3 {
		if !s3_setting.GetS3Setting().Ready() {
			return "", errors.New("S3 存储未启用或配置不完整，无法上传图片")
		}
		return UploadObject(ctx, objectKey, contentType, data)
	}
	return SaveLocalUpload(group, extension, data)
}

// DeleteUploadedImageByURL removes a previously stored image, whichever backend
// produced it. Unknown URLs are ignored so avatars imported from OAuth
// providers never trigger deletions.
func DeleteUploadedImageByURL(ctx context.Context, rawURL string) {
	if IsLocalUploadPath(rawURL) {
		_ = DeleteLocalUpload(rawURL)
		return
	}
	DeleteS3ObjectByURL(ctx, rawURL)
}
