package image_playground_setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowedModelsParsesCommaSeparatedList(t *testing.T) {
	setting := ImagePlaygroundSetting{Models: " gpt-image-1, ,dall-e-3 ,,"}

	assert.Equal(t, []string{"gpt-image-1", "dall-e-3"}, setting.AllowedModels())
}

func TestModelAllowedTreatsEmptyAllowlistAsUnrestricted(t *testing.T) {
	setting := ImagePlaygroundSetting{}

	assert.True(t, setting.ModelAllowed("gpt-image-1"))
}

func TestModelAllowedRejectsModelOutsideAllowlist(t *testing.T) {
	setting := ImagePlaygroundSetting{Models: "gpt-image-1,dall-e-3"}

	assert.True(t, setting.ModelAllowed("dall-e-3"))
	assert.False(t, setting.ModelAllowed("gpt-image-2"))
}

func TestLimitsClampToSupportedRange(t *testing.T) {
	tests := []struct {
		name       string
		setting    ImagePlaygroundSetting
		references int
		images     int
		retention  int
	}{
		{
			name:       "defaults are returned unchanged",
			setting:    ImagePlaygroundSetting{MaxReferenceImages: 4, MaxImagesPerTask: 4, RetentionDays: 7},
			references: 4,
			images:     4,
			retention:  7,
		},
		{
			name:       "negative reference limit becomes zero",
			setting:    ImagePlaygroundSetting{MaxReferenceImages: -3, MaxImagesPerTask: 0, RetentionDays: -1},
			references: 0,
			images:     1,
			retention:  0,
		},
		{
			name:       "overflowing values are clamped",
			setting:    ImagePlaygroundSetting{MaxReferenceImages: 40, MaxImagesPerTask: 40, RetentionDays: 400},
			references: maxMaxReferenceImages,
			images:     maxMaxImagesPerTask,
			retention:  maxRetentionDays,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.references, test.setting.ReferenceImageLimit())
			assert.Equal(t, test.images, test.setting.ImageCountLimit())
			assert.Equal(t, test.retention, test.setting.Retention())
		})
	}
}

func TestValidateModelsAcceptsEmptyAndValidLists(t *testing.T) {
	require.NoError(t, ValidateModels(""))
	require.NoError(t, ValidateModels("gpt-image-1, dall-e-3"))
	require.NoError(t, ValidateModels(" , , "))
}

func TestValidateModelsRejectsWhitespaceInsideName(t *testing.T) {
	err := ValidateModels("gpt image")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "空白字符")
}

func TestValidateModelsRejectsTooManyEntries(t *testing.T) {
	models := strings.Repeat("model,", maxAllowedModels+1)

	err := ValidateModels(models)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "最多允许")
}

func TestValidateModelLimitValues(t *testing.T) {
	assert.NoError(t, ValidateMaxReferenceImages("0"))
	assert.NoError(t, ValidateMaxReferenceImages("16"))
	assert.Error(t, ValidateMaxReferenceImages("17"))
	assert.Error(t, ValidateMaxReferenceImages("abc"))

	assert.NoError(t, ValidateMaxImagesPerTask("1"))
	assert.NoError(t, ValidateMaxImagesPerTask("8"))
	assert.Error(t, ValidateMaxImagesPerTask("0"))
	assert.Error(t, ValidateMaxImagesPerTask("9"))

	assert.NoError(t, ValidateRetentionDays("0"))
	assert.NoError(t, ValidateRetentionDays("90"))
	assert.Error(t, ValidateRetentionDays("-1"))
	assert.Error(t, ValidateRetentionDays("91"))
	assert.Error(t, ValidateRetentionDays("seven"))
}
