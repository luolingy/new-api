package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/image_playground_setting"
)

const (
	imagePlaygroundRequestTimeout = 30 * time.Minute
	imagePlaygroundAssetMaxBytes  = 64 << 20
	imagePlaygroundQueueSize      = 128
	imagePlaygroundWorkers        = 2
	imagePlaygroundTokenTTL       = 3 * 3600
)

var imagePlaygroundQueue = make(chan string, imagePlaygroundQueueSize)

var imagePlaygroundHTTPClient = &http.Client{Timeout: imagePlaygroundRequestTimeout}

// StartImagePlaygroundWorker restores persisted tasks and starts the background
// generator. Tasks keep running while the submitting client is offline.
func StartImagePlaygroundWorker() {
	if err := os.MkdirAll(ImagePlaygroundBaseDir(), 0o755); err != nil {
		common.SysError("failed to create image playground directory: " + err.Error())
		return
	}
	if err := LoadImagePlaygroundTasks(); err != nil {
		common.SysError("failed to load image playground tasks: " + err.Error())
		return
	}
	deleteStaleImagePlaygroundTokens()
	PruneImagePlaygroundTasks(image_playground_setting.GetImagePlaygroundSetting().Retention())
	pending := RecoverImagePlaygroundTasks()
	// Generators talk to this process over loopback, so they must not start
	// before the HTTP listener is reachable. New submissions are buffered by the
	// queue until then.
	go func() {
		waitForImagePlaygroundServer()
		for range imagePlaygroundWorkers {
			go imagePlaygroundWorkerLoop()
		}
		for _, taskId := range pending {
			ImagePlaygroundEnqueue(taskId)
		}
	}()
	go imagePlaygroundPruneLoop()
}

// waitForImagePlaygroundServer blocks until the local HTTP port accepts
// connections, so a restart does not fail recovered tasks with a connection
// refused error.
func waitForImagePlaygroundServer() {
	address := "127.0.0.1:" + imagePlaygroundServerPort()
	for range 600 {
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	common.SysError("image playground: local server port never became reachable, queued tasks were not started")
}

// ImagePlaygroundEnqueue schedules a queued task for generation.
func ImagePlaygroundEnqueue(taskId string) {
	if taskId == "" {
		return
	}
	select {
	case imagePlaygroundQueue <- taskId:
	default:
		common.SysError("image playground queue is full, task " + taskId + " was not scheduled")
	}
}

func imagePlaygroundPruneLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		PruneImagePlaygroundTasks(image_playground_setting.GetImagePlaygroundSetting().Retention())
	}
}

func imagePlaygroundWorkerLoop() {
	for taskId := range imagePlaygroundQueue {
		runImagePlaygroundTask(taskId)
	}
}

func runImagePlaygroundTask(taskId string) {
	task, ok := ClaimImagePlaygroundTask(taskId)
	if !ok {
		return
	}
	err := executeImagePlaygroundTask(task)
	if err != nil {
		task.Status = ImagePlaygroundStatusFailed
		task.Error = err.Error()
		common.SysError(fmt.Sprintf("image playground task %s failed: %v", task.Id, err))
	} else {
		task.Status = ImagePlaygroundStatusSucceeded
		task.Error = ""
	}
	task.FinishedAt = time.Now().Unix()
	RemoveImagePlaygroundReferences(task)
	RemoveImagePlaygroundPartial(task)
	if saveErr := SaveImagePlaygroundTask(task); saveErr != nil {
		common.SysError("failed to save image playground task result: " + saveErr.Error())
	}
}

func executeImagePlaygroundTask(task *ImagePlaygroundTask) error {
	body, contentType, err := buildImagePlaygroundRequestBody(task)
	if err != nil {
		return err
	}
	token, err := createImagePlaygroundToken(task)
	if err != nil {
		return fmt.Errorf("创建任务凭证失败：%w", err)
	}
	defer func() {
		if deleteErr := token.Delete(); deleteErr != nil {
			common.SysError("failed to delete image playground token: " + deleteErr.Error())
		}
	}()

	requestPath := "/v1/images/generations"
	if len(task.ReferenceFiles) > 0 {
		requestPath = "/v1/images/edits"
	}
	endpoint := "http://127.0.0.1:" + imagePlaygroundServerPort() + requestPath
	ctx, cancel := context.WithTimeout(context.Background(), imagePlaygroundRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token.Key)
	request.Header.Set("Content-Type", contentType)
	accept := "application/json"
	if task.Stream {
		accept = "text/event-stream"
	}
	request.Header.Set("Accept", accept)

	response, err := imagePlaygroundHTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("上游请求失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(io.LimitReader(response.Body, imagePlaygroundAssetMaxBytes))
		return fmt.Errorf("%s", extractImagePlaygroundError(errorBody, response.StatusCode))
	}
	if isImagePlaygroundEventStream(response) {
		assets, streamErr := consumeImagePlaygroundStream(task, response.Body)
		if streamErr != nil {
			return streamErr
		}
		task.Assets = assets
		return nil
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, imagePlaygroundAssetMaxBytes))
	if err != nil {
		return fmt.Errorf("读取上游响应失败：%w", err)
	}
	assets, err := storeImagePlaygroundResponse(task, responseBody)
	if err != nil {
		return err
	}
	task.Assets = assets
	return nil
}

// imagePlaygroundOptionalFields lists the optional upstream parameters that are
// only forwarded when the caller chose a non-default value.
func imagePlaygroundOptionalFields(task *ImagePlaygroundTask) map[string]string {
	fields := map[string]string{}
	if task.Size != "" && task.Size != "auto" {
		fields["size"] = task.Size
	}
	if task.Quality != "" && task.Quality != "auto" {
		fields["quality"] = task.Quality
	}
	if task.Background != "" && task.Background != "auto" {
		fields["background"] = task.Background
	}
	if task.Moderation != "" && task.Moderation != "auto" {
		fields["moderation"] = task.Moderation
	}
	if task.ResponseFormat != "" {
		fields["response_format"] = task.ResponseFormat
	}
	if task.Stream {
		fields["stream"] = "true"
		fields["partial_images"] = strconv.Itoa(task.PartialImages)
	}
	return fields
}

func buildImagePlaygroundRequestBody(task *ImagePlaygroundTask) ([]byte, string, error) {
	optionalFields := imagePlaygroundOptionalFields(task)
	multipartBody := len(task.ReferenceFiles) > 0
	if !multipartBody {
		payload := map[string]any{
			"model":         task.Model,
			"prompt":        task.Prompt,
			"output_format": task.OutputFormat,
		}
		for key, value := range optionalFields {
			payload[key] = value
		}
		// The JSON API expects a boolean, a string works for multipart only.
		if task.Stream {
			payload["stream"] = true
			payload["partial_images"] = task.PartialImages
		}
		if task.N > 1 {
			payload["n"] = task.N
		}
		body, err := common.Marshal(payload)
		if err != nil {
			return nil, "", err
		}
		return body, "application/json", nil
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	fields := map[string]string{
		"model":         task.Model,
		"prompt":        task.Prompt,
		"output_format": task.OutputFormat,
	}
	for key, value := range optionalFields {
		fields[key] = value
	}
	if task.N > 1 {
		fields["n"] = strconv.Itoa(task.N)
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	for index, name := range task.ReferenceFiles {
		data, err := ReadImagePlaygroundReference(task, name)
		if err != nil {
			return nil, "", fmt.Errorf("读取参考图失败：%w", err)
		}
		mimeType := detectImageMimeType(data)
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="input-%d%s"`, index+1, imageMimeExtension(mimeType)))
		header.Set("Content-Type", mimeType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func createImagePlaygroundToken(task *ImagePlaygroundTask) (*model.Token, error) {
	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	token := &model.Token{
		UserId:             task.UserId,
		Key:                key,
		Status:             common.TokenStatusEnabled,
		Name:               "image-playground-" + task.Id,
		CreatedTime:        now,
		ExpiredTime:        now + imagePlaygroundTokenTTL,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: true,
		ModelLimits:        task.Model,
		Group:              task.Group,
	}
	if err := token.Insert(); err != nil {
		return nil, err
	}
	return token, nil
}

// deleteStaleImagePlaygroundTokens removes task credentials left behind by an
// interrupted run so they can never be reused.
func deleteStaleImagePlaygroundTokens() {
	tokens := make([]*model.Token, 0, 16)
	if err := model.DB.
		Where("name LIKE ?", "image-playground-%").
		Find(&tokens).Error; err != nil {
		common.SysError("failed to load stale image playground tokens: " + err.Error())
		return
	}
	for _, token := range tokens {
		if err := token.Delete(); err != nil {
			common.SysError("failed to delete stale image playground token: " + err.Error())
		}
	}
}

func imagePlaygroundServerPort() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return port
	}
	if common.Port != nil {
		return strconv.Itoa(*common.Port)
	}
	return "3000"
}

type imagePlaygroundResponseItem struct {
	Url           string `json:"url"`
	B64Json       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}

type imagePlaygroundResponsePayload struct {
	Data []imagePlaygroundResponseItem `json:"data"`
}

func storeImagePlaygroundResponse(task *ImagePlaygroundTask, payload []byte) ([]ImagePlaygroundAsset, error) {
	parsed := imagePlaygroundResponsePayload{}
	if err := common.Unmarshal(payload, &parsed); err != nil {
		return nil, fmt.Errorf("解析上游响应失败：%w", err)
	}
	if len(parsed.Data) == 0 {
		return nil, fmt.Errorf("%s", extractImagePlaygroundError(payload, http.StatusOK))
	}
	return storeImagePlaygroundItems(task, parsed.Data)
}

func storeImagePlaygroundItems(task *ImagePlaygroundTask, items []imagePlaygroundResponseItem) ([]ImagePlaygroundAsset, error) {
	assets := make([]ImagePlaygroundAsset, 0, len(items))
	for index, item := range items {
		data, err := loadImagePlaygroundImageBytes(item.Url, item.B64Json)
		if err != nil {
			return nil, fmt.Errorf("第 %d 张图片下载失败：%w", index+1, err)
		}
		asset, err := StoreImagePlaygroundAsset(task, index, data, item.RevisedPrompt)
		if err != nil {
			return nil, fmt.Errorf("保存图片失败：%w", err)
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func loadImagePlaygroundImageBytes(url string, b64 string) ([]byte, error) {
	if b64 != "" {
		return decodeImagePlaygroundBase64(b64)
	}
	if url == "" {
		return nil, fmt.Errorf("上游未返回图片数据")
	}
	if strings.HasPrefix(url, "data:") {
		_, encoded, found := strings.Cut(url, ",")
		if !found {
			return nil, fmt.Errorf("图片 data URL 格式不正确")
		}
		return decodeImagePlaygroundBase64(encoded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := imagePlaygroundHTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片地址返回状态码 %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, imagePlaygroundAssetMaxBytes))
}

func decodeImagePlaygroundBase64(value string) ([]byte, error) {
	if _, encoded, found := strings.Cut(value, ","); found && strings.HasPrefix(value, "data:") {
		value = encoded
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func extractImagePlaygroundError(payload []byte, statusCode int) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(payload, &parsed); err == nil {
		if message := strings.TrimSpace(parsed.Error.Message); message != "" {
			return message
		}
		if message := strings.TrimSpace(parsed.Message); message != "" {
			return message
		}
	}
	snippet := strings.TrimSpace(string(payload))
	if len(snippet) > 300 {
		snippet = snippet[:300] + "..."
	}
	if snippet == "" {
		return fmt.Sprintf("上游返回状态码 %d，未返回错误信息", statusCode)
	}
	return fmt.Sprintf("上游返回状态码 %d：%s", statusCode, snippet)
}

func isImagePlaygroundEventStream(response *http.Response) bool {
	return strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
}

type imagePlaygroundStreamEvent struct {
	Type              string `json:"type"`
	B64Json           string `json:"b64_json"`
	Url               string `json:"url"`
	RevisedPrompt     string `json:"revised_prompt"`
	PartialImageIndex int    `json:"partial_image_index"`
	Data              []any  `json:"data"`
	Message           string `json:"message"`
	Error             *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// consumeImagePlaygroundStream reads an upstream image SSE stream, persisting
// progressive previews as they arrive and returning the finished images. It
// accepts both the event style used by OpenAI image streaming
// (image_generation.partial_image / .completed / image.generation.result) and a
// plain JSON payload wrapped in a data frame.
func consumeImagePlaygroundStream(task *ImagePlaygroundTask, body io.Reader) ([]ImagePlaygroundAsset, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), imagePlaygroundAssetMaxBytes)
	dataLines := make([]string, 0, 4)
	completed := make([]imagePlaygroundResponseItem, 0, 4)
	var finalPayload string
	eventCount := 0

	handleEvent := func(payload string) error {
		event := imagePlaygroundStreamEvent{}
		if err := common.Unmarshal([]byte(payload), &event); err != nil {
			// Unknown non-JSON frame, ignore it instead of failing the task.
			return nil
		}
		eventCount++
		if event.Error != nil && strings.TrimSpace(event.Error.Message) != "" {
			return fmt.Errorf("%s", event.Error.Message)
		}
		if strings.HasSuffix(event.Type, ".failed") {
			if message := strings.TrimSpace(event.Message); message != "" {
				return fmt.Errorf("%s", message)
			}
			return fmt.Errorf("流式生成失败")
		}
		if strings.HasSuffix(event.Type, ".partial_image") {
			if event.B64Json == "" {
				return nil
			}
			data, err := decodeImagePlaygroundBase64(event.B64Json)
			if err != nil {
				return nil
			}
			partial, err := StoreImagePlaygroundPartial(task, event.PartialImageIndex, data)
			if err != nil {
				common.SysError("failed to store image playground partial image: " + err.Error())
				return nil
			}
			task.Partial = &partial
			if saveErr := SaveImagePlaygroundTask(task); saveErr != nil {
				common.SysError("failed to save image playground partial state: " + saveErr.Error())
			}
			return nil
		}
		if event.B64Json != "" || event.Url != "" {
			completed = append(completed, imagePlaygroundResponseItem{
				B64Json:       event.B64Json,
				Url:           event.Url,
				RevisedPrompt: event.RevisedPrompt,
			})
			return nil
		}
		if len(event.Data) > 0 {
			encoded, err := common.Marshal(map[string]any{"data": event.Data})
			if err == nil {
				finalPayload = string(encoded)
			}
		}
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if len(dataLines) == 0 {
				continue
			}
			payload := strings.Join(dataLines, "\n")
			dataLines = dataLines[:0]
			if strings.TrimSpace(payload) == "[DONE]" {
				break
			}
			if err := handleEvent(payload); err != nil {
				return nil, err
			}
			continue
		}
		value, found := strings.CutPrefix(line, "data:")
		if !found {
			continue
		}
		dataLines = append(dataLines, strings.TrimPrefix(value, " "))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取流式响应失败：%w", err)
	}
	if len(dataLines) > 0 {
		payload := strings.Join(dataLines, "\n")
		if strings.TrimSpace(payload) != "[DONE]" {
			if err := handleEvent(payload); err != nil {
				return nil, err
			}
		}
	}
	if eventCount == 0 {
		return nil, fmt.Errorf("流式响应中没有有效的图片事件")
	}
	if finalPayload != "" {
		return storeImagePlaygroundResponse(task, []byte(finalPayload))
	}
	if len(completed) > 0 {
		return storeImagePlaygroundItems(task, completed)
	}
	return nil, fmt.Errorf("流式响应已结束但未返回图片数据")
}
