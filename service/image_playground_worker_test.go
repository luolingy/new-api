package service

import (
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A 1x1 PNG used as the upstream payload in streaming tests.
const imagePlaygroundTestPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg=="

func newImagePlaygroundTestTask(t *testing.T) *ImagePlaygroundTask {
	t.Helper()
	t.Setenv("IMAGE_PLAYGROUND_DIR", t.TempDir())
	return &ImagePlaygroundTask{
		Id:            "task-1",
		UserId:        7,
		Status:        ImagePlaygroundStatusRunning,
		Model:         "gpt-image-1",
		Prompt:        "a cat",
		Size:          "1024x1024",
		Quality:       "high",
		OutputFormat:  "png",
		N:             2,
		Stream:        true,
		PartialImages: 2,
	}
}

func TestConsumeImagePlaygroundStreamStoresPreviewAndFinalImage(t *testing.T) {
	task := newImagePlaygroundTestTask(t)
	stream := strings.Join([]string{
		`data: {"type":"image_generation.partial_image","partial_image_index":0,"b64_json":"` + imagePlaygroundTestPNGBase64 + `"}`,
		"",
		`data: {"type":"image_generation.completed","b64_json":"` + imagePlaygroundTestPNGBase64 + `","revised_prompt":"a photo of a cat"}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	assets, err := consumeImagePlaygroundStream(task, strings.NewReader(stream))

	require.NoError(t, err)
	require.Len(t, assets, 1)
	assert.Equal(t, "image/png", assets[0].MimeType)
	assert.Equal(t, "a photo of a cat", assets[0].RevisedPrompt)
	require.NotNil(t, task.Partial)
	assert.Equal(t, 0, task.Partial.Index)

	task.Assets = assets
	data, mimeType, err := ReadImagePlaygroundAsset(task, 0)
	require.NoError(t, err)
	assert.Equal(t, "image/png", mimeType)
	assert.NotEmpty(t, data)
}

func TestConsumeImagePlaygroundStreamAcceptsWrappedJSONData(t *testing.T) {
	task := newImagePlaygroundTestTask(t)
	stream := "data: {\"data\":[{\"b64_json\":\"" + imagePlaygroundTestPNGBase64 + "\"}]}\n\ndata: [DONE]\n\n"

	assets, err := consumeImagePlaygroundStream(task, strings.NewReader(stream))

	require.NoError(t, err)
	require.Len(t, assets, 1)
}

func TestConsumeImagePlaygroundStreamReportsUpstreamError(t *testing.T) {
	task := newImagePlaygroundTestTask(t)
	stream := "data: {\"type\":\"image_generation.failed\",\"message\":\"upstream rejected the prompt\"}\n\n"

	_, err := consumeImagePlaygroundStream(task, strings.NewReader(stream))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream rejected the prompt")
}

func TestConsumeImagePlaygroundStreamFailsWithoutImageEvents(t *testing.T) {
	task := newImagePlaygroundTestTask(t)

	_, err := consumeImagePlaygroundStream(task, strings.NewReader("data: {\"type\":\"ping\"}\n\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "未返回图片数据")
}

func TestBuildImagePlaygroundRequestBodySendsStreamOptions(t *testing.T) {
	task := newImagePlaygroundTestTask(t)
	task.ResponseFormat = "b64_json"

	body, contentType, err := buildImagePlaygroundRequestBody(task)

	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	payload := string(body)
	assert.Contains(t, payload, `"stream":true`)
	assert.Contains(t, payload, `"partial_images":2`)
	assert.Contains(t, payload, `"response_format":"b64_json"`)
	assert.Contains(t, payload, `"size":"1024x1024"`)
	assert.Contains(t, payload, `"n":2`)
}

func TestBuildImagePlaygroundRequestBodyAddsStreamFieldsToMultipart(t *testing.T) {
	task := newImagePlaygroundTestTask(t)
	task.ResponseFormat = "b64_json"
	name, err := WriteImagePlaygroundReference(task, 0, ".png", []byte{0x89, 0x50, 0x4e, 0x47})
	require.NoError(t, err)
	task.ReferenceFiles = []string{name}

	body, contentType, err := buildImagePlaygroundRequestBody(task)

	require.NoError(t, err)
	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	form, err := reader.ReadForm(1 << 20)
	require.NoError(t, err)
	assert.Equal(t, "true", form.Value["stream"][0])
	assert.Equal(t, "2", form.Value["partial_images"][0])
	assert.Equal(t, "b64_json", form.Value["response_format"][0])
	require.Len(t, form.File["image[]"], 1)
}

func TestLoadImagePlaygroundImageBytesRejectsEmptyItem(t *testing.T) {
	_, err := loadImagePlaygroundImageBytes("", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "未返回图片数据")
}
