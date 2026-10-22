package image_playground_setting

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ImagePlaygroundSetting controls the offline image generation workspace. Tasks
// submitted from the playground run on the server, so the client may close the
// page (or lose connectivity) while the image is still being generated.
type ImagePlaygroundSetting struct {
	Enabled            bool   `json:"enabled"`
	Models             string `json:"models"`
	MaxReferenceImages int    `json:"max_reference_images"`
	MaxImagesPerTask   int    `json:"max_images_per_task"`
	RetentionDays      int    `json:"retention_days"`
}

const (
	minMaxReferenceImages = 0
	maxMaxReferenceImages = 16
	minMaxImagesPerTask   = 1
	maxMaxImagesPerTask   = 8
	maxRetentionDays      = 90
	maxAllowedModels      = 100
)

var defaultImagePlaygroundSetting = ImagePlaygroundSetting{
	Enabled:            true,
	MaxReferenceImages: 4,
	MaxImagesPerTask:   4,
	RetentionDays:      7,
}

func init() {
	config.GlobalConfig.Register("image_playground", &defaultImagePlaygroundSetting)
}

// GetImagePlaygroundSetting returns the live configuration. The config manager
// replaces the struct contents in place, so callers must not retain it.
func GetImagePlaygroundSetting() *ImagePlaygroundSetting {
	return &defaultImagePlaygroundSetting
}

// AllowedModels returns the configured model allowlist. An empty list means
// every model the user can reach is allowed.
func (s *ImagePlaygroundSetting) AllowedModels() []string {
	models := make([]string, 0, 8)
	for _, entry := range strings.Split(s.Models, ",") {
		if model := strings.TrimSpace(entry); model != "" {
			models = append(models, model)
		}
	}
	return models
}

// ModelAllowed reports whether the model may be used by the playground.
func (s *ImagePlaygroundSetting) ModelAllowed(model string) bool {
	allowed := s.AllowedModels()
	if len(allowed) == 0 {
		return true
	}
	for _, entry := range allowed {
		if entry == model {
			return true
		}
	}
	return false
}

// ReferenceImageLimit returns the effective reference image limit.
func (s *ImagePlaygroundSetting) ReferenceImageLimit() int {
	if s.MaxReferenceImages < minMaxReferenceImages {
		return 0
	}
	if s.MaxReferenceImages > maxMaxReferenceImages {
		return maxMaxReferenceImages
	}
	return s.MaxReferenceImages
}

// ImageCountLimit returns the effective per-task image count limit.
func (s *ImagePlaygroundSetting) ImageCountLimit() int {
	if s.MaxImagesPerTask < minMaxImagesPerTask {
		return minMaxImagesPerTask
	}
	if s.MaxImagesPerTask > maxMaxImagesPerTask {
		return maxMaxImagesPerTask
	}
	return s.MaxImagesPerTask
}

// Retention returns the effective number of days a finished task is kept. Zero
// keeps tasks until the user deletes them.
func (s *ImagePlaygroundSetting) Retention() int {
	if s.RetentionDays < 0 {
		return 0
	}
	if s.RetentionDays > maxRetentionDays {
		return maxRetentionDays
	}
	return s.RetentionDays
}

// ValidateModels accepts an empty value (all models) or a comma separated list
// of model identifiers.
func ValidateModels(value string) error {
	entries := strings.Split(value, ",")
	count := 0
	for _, entry := range entries {
		model := strings.TrimSpace(entry)
		if model == "" {
			continue
		}
		count++
		if count > maxAllowedModels {
			return fmt.Errorf("模型列表最多允许 %d 项", maxAllowedModels)
		}
		if len(model) > 128 {
			return fmt.Errorf("模型名称过长：%s", model)
		}
		for _, r := range model {
			if r <= ' ' || r == 0x7f {
				return fmt.Errorf("模型名称不能包含空白字符或控制字符")
			}
		}
	}
	return nil
}

// ValidateMaxReferenceImages bounds the per-task reference image count.
func ValidateMaxReferenceImages(value string) error {
	count, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("参考图上限必须是整数")
	}
	if count < minMaxReferenceImages || count > maxMaxReferenceImages {
		return fmt.Errorf("参考图上限必须在 %d-%d 之间", minMaxReferenceImages, maxMaxReferenceImages)
	}
	return nil
}

// ValidateMaxImagesPerTask bounds the per-task generated image count.
func ValidateMaxImagesPerTask(value string) error {
	count, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("单次生图张数上限必须是整数")
	}
	if count < minMaxImagesPerTask || count > maxMaxImagesPerTask {
		return fmt.Errorf("单次生图张数上限必须在 %d-%d 之间", minMaxImagesPerTask, maxMaxImagesPerTask)
	}
	return nil
}

// ValidateRetentionDays bounds how long finished tasks are kept on disk.
func ValidateRetentionDays(value string) error {
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("保留天数必须是整数")
	}
	if days < 0 || days > maxRetentionDays {
		return fmt.Errorf("保留天数必须在 0-%d 之间", maxRetentionDays)
	}
	return nil
}
