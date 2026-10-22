package controller

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/image_playground_setting"

	"github.com/gin-gonic/gin"
)

const (
	imagePlaygroundMaxPromptLength   = 4000
	imagePlaygroundMaxReferenceBytes = 10 << 20
	imagePlaygroundMaxReferenceTotal = 40 << 20
	imagePlaygroundMaxActiveTasks    = 3
	imagePlaygroundMaxSizeEdge       = 4096
	imagePlaygroundMinSizeEdge       = 64
	imagePlaygroundMaxPartialImages  = 3
	imagePlaygroundDefaultPartials   = 2
)

var imagePlaygroundSizePattern = regexp.MustCompile(`^[0-9]{2,5}x[0-9]{2,5}$`)

var imagePlaygroundQualityValues = map[string]bool{
	"auto":   true,
	"low":    true,
	"medium": true,
	"high":   true,
	"xhigh":  true,
	"max":    true,
}

var imagePlaygroundOutputFormats = map[string]bool{
	"png":  true,
	"jpeg": true,
	"webp": true,
}

var imagePlaygroundBackgrounds = map[string]bool{
	"auto":        true,
	"transparent": true,
	"opaque":      true,
}

var imagePlaygroundModerations = map[string]bool{
	"auto": true,
	"low":  true,
}

func imagePlaygroundAccessDisabled(userId int) bool {
	setting, err := model.GetUserSetting(userId, false)
	if err != nil {
		return false
	}
	return setting.ImagePlaygroundDisabled
}

// GetImagePlaygroundConfig reports whether the workspace is usable for the
// current user and which limits apply.
func GetImagePlaygroundConfig(c *gin.Context) {
	userId := c.GetInt("id")
	setting := image_playground_setting.GetImagePlaygroundSetting()
	disabled := imagePlaygroundAccessDisabled(userId)
	message := ""
	if !setting.Enabled {
		message = "管理员已关闭在线生图功能"
	} else if disabled {
		message = "当前账号已被禁止使用在线生图功能"
	}
	common.ApiSuccess(c, gin.H{
		"enabled":              setting.Enabled && !disabled,
		"globally_enabled":     setting.Enabled,
		"restricted":           disabled,
		"message":              message,
		"max_reference_images": setting.ReferenceImageLimit(),
		"max_images_per_task":  setting.ImageCountLimit(),
		"allowed_models":       setting.AllowedModels(),
		"retention_days":       setting.Retention(),
	})
}

// CreateImagePlaygroundTask accepts a generation request and queues it for
// server-side execution, so the client may disconnect immediately.
func CreateImagePlaygroundTask(c *gin.Context) {
	userId := c.GetInt("id")
	setting := image_playground_setting.GetImagePlaygroundSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "管理员已关闭在线生图功能")
		return
	}
	if imagePlaygroundAccessDisabled(userId) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "当前账号已被禁止使用在线生图功能"})
		return
	}
	// Limit the whole request before any multipart field is parsed; the default
	// Gin/Go limits would otherwise be applied first by c.PostForm.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, imagePlaygroundMaxReferenceTotal)
	if err := c.Request.ParseMultipartForm(imagePlaygroundMaxReferenceTotal); err != nil {
		common.ApiErrorMsg(c, "请求格式不正确")
		return
	}

	modelName := strings.TrimSpace(c.PostForm("model"))
	if modelName == "" {
		common.ApiErrorMsg(c, "请选择生图模型")
		return
	}
	if !setting.ModelAllowed(modelName) {
		common.ApiErrorMsg(c, "该模型未开放给在线生图使用")
		return
	}
	prompt := strings.TrimSpace(c.PostForm("prompt"))
	if prompt == "" {
		common.ApiErrorMsg(c, "请输入提示词")
		return
	}
	if len(prompt) > imagePlaygroundMaxPromptLength {
		common.ApiErrorMsg(c, fmt.Sprintf("提示词不能超过 %d 个字符", imagePlaygroundMaxPromptLength))
		return
	}

	imageCount := 1
	if raw := strings.TrimSpace(c.PostForm("n")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			common.ApiErrorMsg(c, "生图张数必须是整数")
			return
		}
		imageCount = parsed
	}
	if imageCount < 1 || imageCount > setting.ImageCountLimit() {
		common.ApiErrorMsg(c, fmt.Sprintf("生图张数必须在 1-%d 之间", setting.ImageCountLimit()))
		return
	}

	size := strings.TrimSpace(c.PostForm("size"))
	if size == "" {
		size = "auto"
	}
	if size != "auto" {
		if !imagePlaygroundSizePattern.MatchString(size) {
			common.ApiErrorMsg(c, "尺寸格式必须为 宽x高，例如 1024x1024")
			return
		}
		widthText, heightText, _ := strings.Cut(size, "x")
		width, _ := strconv.Atoi(widthText)
		height, _ := strconv.Atoi(heightText)
		if width < imagePlaygroundMinSizeEdge || width > imagePlaygroundMaxSizeEdge ||
			height < imagePlaygroundMinSizeEdge || height > imagePlaygroundMaxSizeEdge {
			common.ApiErrorMsg(c, fmt.Sprintf("尺寸边长必须在 %d-%d 之间", imagePlaygroundMinSizeEdge, imagePlaygroundMaxSizeEdge))
			return
		}
	}

	quality := strings.TrimSpace(c.PostForm("quality"))
	if quality == "" {
		quality = "auto"
	}
	if !imagePlaygroundQualityValues[quality] {
		common.ApiErrorMsg(c, "画质参数不合法")
		return
	}

	background := strings.TrimSpace(c.PostForm("background"))
	if background == "" {
		background = "auto"
	}
	if !imagePlaygroundBackgrounds[background] {
		common.ApiErrorMsg(c, "背景参数不合法")
		return
	}

	moderation := strings.TrimSpace(c.PostForm("moderation"))
	if moderation == "" {
		moderation = "auto"
	}
	if !imagePlaygroundModerations[moderation] {
		common.ApiErrorMsg(c, "审核参数不合法")
		return
	}

	outputFormat := strings.TrimSpace(c.PostForm("output_format"))
	if outputFormat == "" {
		outputFormat = "png"
	}
	if !imagePlaygroundOutputFormats[outputFormat] {
		common.ApiErrorMsg(c, "输出格式不合法")
		return
	}

	// Streaming previews need the upstream to emit SSE partial frames; without
	// the option the task simply waits for the finished images.
	stream := false
	if raw := strings.TrimSpace(c.PostForm("stream")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			common.ApiErrorMsg(c, "流式参数不合法")
			return
		}
		stream = parsed
	}
	partialImages := 0
	if stream {
		partialImages = imagePlaygroundDefaultPartials
		if raw := strings.TrimSpace(c.PostForm("partial_images")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 0 || parsed > imagePlaygroundMaxPartialImages {
				common.ApiErrorMsg(c, fmt.Sprintf("流式预览帧数必须在 0-%d 之间", imagePlaygroundMaxPartialImages))
				return
			}
			partialImages = parsed
		}
	}

	responseFormat := strings.TrimSpace(c.PostForm("response_format"))
	if responseFormat == "" {
		responseFormat = "b64_json"
	}
	if responseFormat != "b64_json" && responseFormat != "url" {
		common.ApiErrorMsg(c, "返回格式不合法")
		return
	}

	userGroup, err := model.GetUserGroup(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	group := strings.TrimSpace(c.PostForm("group"))
	if group == "" {
		group = userGroup
	}
	if group != userGroup && !service.GroupInUserUsableGroups(userGroup, group) {
		common.ApiErrorMsg(c, "无权使用该分组")
		return
	}

	if quota, quotaErr := model.GetUserQuota(userId, false); quotaErr == nil && quota <= 0 {
		common.ApiErrorMsg(c, "额度不足，请先充值")
		return
	}

	if service.CountActiveImagePlaygroundTasks(userId) >= imagePlaygroundMaxActiveTasks {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "队列中的生图任务过多，请稍后再试"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, imagePlaygroundMaxReferenceTotal)
	if err := c.Request.ParseMultipartForm(imagePlaygroundMaxReferenceTotal); err != nil {
		common.ApiErrorMsg(c, "请求格式不正确")
		return
	}
	var files []*multipartFile
	if c.Request.MultipartForm != nil {
		headers := c.Request.MultipartForm.File["references"]
		if len(headers) > setting.ReferenceImageLimit() {
			common.ApiErrorMsg(c, fmt.Sprintf("参考图最多 %d 张", setting.ReferenceImageLimit()))
			return
		}
		for _, header := range headers {
			if header.Size > imagePlaygroundMaxReferenceBytes {
				common.ApiErrorMsg(c, fmt.Sprintf("单张参考图不能超过 %d MB", imagePlaygroundMaxReferenceBytes>>20))
				return
			}
			file, openErr := header.Open()
			if openErr != nil {
				common.ApiErrorMsg(c, "读取参考图失败")
				return
			}
			data, readErr := io.ReadAll(io.LimitReader(file, imagePlaygroundMaxReferenceBytes+1))
			file.Close()
			if readErr != nil {
				common.ApiErrorMsg(c, "读取参考图失败")
				return
			}
			if len(data) > imagePlaygroundMaxReferenceBytes {
				common.ApiErrorMsg(c, fmt.Sprintf("单张参考图不能超过 %d MB", imagePlaygroundMaxReferenceBytes>>20))
				return
			}
			contentType := http.DetectContentType(data)
			if !strings.HasPrefix(contentType, "image/") {
				common.ApiErrorMsg(c, "参考图必须是图片文件")
				return
			}
			files = append(files, &multipartFile{data: data, extension: imagePlaygroundExtensionFor(contentType)})
		}
	}
	if len(files) > setting.ReferenceImageLimit() {
		common.ApiErrorMsg(c, fmt.Sprintf("参考图最多 %d 张", setting.ReferenceImageLimit()))
		return
	}

	task := &service.ImagePlaygroundTask{
		Id:             common.GetUUID(),
		UserId:         userId,
		Status:         service.ImagePlaygroundStatusQueued,
		Prompt:         prompt,
		Model:          modelName,
		Group:          group,
		Size:           size,
		Quality:        quality,
		Background:     background,
		Moderation:     moderation,
		OutputFormat:   outputFormat,
		Stream:         stream,
		PartialImages:  partialImages,
		ResponseFormat: responseFormat,
		N:              imageCount,
		CreatedAt:      common.GetTimestamp(),
	}
	if err := service.CreateImagePlaygroundTask(task); err != nil {
		common.ApiError(c, err)
		return
	}
	for index, file := range files {
		name, writeErr := service.WriteImagePlaygroundReference(task, index, file.extension, file.data)
		if writeErr != nil {
			service.DeleteImagePlaygroundTask(userId, task.Id)
			common.ApiError(c, writeErr)
			return
		}
		task.ReferenceFiles = append(task.ReferenceFiles, name)
	}
	if len(task.ReferenceFiles) > 0 {
		if err := service.SaveImagePlaygroundTask(task); err != nil {
			service.DeleteImagePlaygroundTask(userId, task.Id)
			common.ApiError(c, err)
			return
		}
	}
	service.ImagePlaygroundEnqueue(task.Id)
	common.ApiSuccess(c, imagePlaygroundTaskResponse(task))
}

type multipartFile struct {
	data      []byte
	extension string
}

func imagePlaygroundExtensionFor(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func imagePlaygroundTaskResponse(task *service.ImagePlaygroundTask) gin.H {
	images := make([]gin.H, 0, len(task.Assets))
	for index, asset := range task.Assets {
		images = append(images, gin.H{
			"index":          index,
			"url":            fmt.Sprintf("/api/image-playground/tasks/%s/assets/%d", task.Id, index),
			"mime_type":      asset.MimeType,
			"revised_prompt": asset.RevisedPrompt,
		})
	}
	var partial gin.H
	if task.Partial != nil {
		partial = gin.H{
			"index":      task.Partial.Index,
			"updated_at": task.Partial.UpdatedAt,
			"url":        fmt.Sprintf("/api/image-playground/tasks/%s/partial", task.Id),
		}
	}
	return gin.H{
		"id":              task.Id,
		"status":          task.Status,
		"prompt":          task.Prompt,
		"model":           task.Model,
		"group":           task.Group,
		"size":            task.Size,
		"quality":         task.Quality,
		"background":      task.Background,
		"moderation":      task.Moderation,
		"output_format":   task.OutputFormat,
		"stream":          task.Stream,
		"partial_images":  task.PartialImages,
		"response_format": task.ResponseFormat,
		"n":               task.N,
		"reference_count": len(task.ReferenceFiles),
		"images":          images,
		"partial":         partial,
		"error":           task.Error,
		"created_at":      task.CreatedAt,
		"started_at":      task.StartedAt,
		"finished_at":     task.FinishedAt,
	}
}

// ListImagePlaygroundTasks returns the newest tasks of the current user.
func ListImagePlaygroundTasks(c *gin.Context) {
	userId := c.GetInt("id")
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		limit, _ = strconv.Atoi(raw)
	}
	tasks := service.ListImagePlaygroundTasks(userId, limit)
	result := make([]gin.H, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, imagePlaygroundTaskResponse(task))
	}
	common.ApiSuccess(c, result)
}

// GetImagePlaygroundTask returns one task of the current user.
func GetImagePlaygroundTask(c *gin.Context) {
	userId := c.GetInt("id")
	task, ok := service.GetImagePlaygroundTask(userId, c.Param("task_id"))
	if !ok {
		common.ApiErrorMsg(c, "任务不存在")
		return
	}
	common.ApiSuccess(c, imagePlaygroundTaskResponse(task))
}

// DeleteImagePlaygroundTask removes a task and its generated images.
func DeleteImagePlaygroundTask(c *gin.Context) {
	userId := c.GetInt("id")
	if !service.DeleteImagePlaygroundTask(userId, c.Param("task_id")) {
		common.ApiErrorMsg(c, "任务不存在")
		return
	}
	common.ApiSuccess(c, nil)
}

// ServeImagePlaygroundAsset streams a generated image of the current user.
func ServeImagePlaygroundAsset(c *gin.Context) {
	userId := c.GetInt("id")
	task, ok := service.GetImagePlaygroundTask(userId, c.Param("task_id"))
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	data, mimeType, err := service.ReadImagePlaygroundAsset(task, index)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mimeType, data)
}

// ServeImagePlaygroundPartial streams the newest progressive preview of a
// running task. The bytes change on every frame, so it is never cached.
func ServeImagePlaygroundPartial(c *gin.Context) {
	userId := c.GetInt("id")
	task, ok := service.GetImagePlaygroundTask(userId, c.Param("task_id"))
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	data, mimeType, err := service.ReadImagePlaygroundPartial(task)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mimeType, data)
}

// AdminGetImagePlaygroundAccess returns whether the playground is disabled for
// one user.
func AdminGetImagePlaygroundAccess(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "用户 ID 不合法")
		return
	}
	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"disabled": user.GetSetting().ImagePlaygroundDisabled})
}

// AdminSetImagePlaygroundAccess enables or disables the playground for one user.
func AdminSetImagePlaygroundAccess(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "用户 ID 不合法")
		return
	}
	var request struct {
		Disabled *bool `json:"disabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Disabled == nil {
		common.ApiErrorMsg(c, "请求参数不正确")
		return
	}
	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user.Role == common.RoleRootUser {
		common.ApiErrorMsg(c, "不能限制超级管理员")
		return
	}
	setting := user.GetSetting()
	if setting.ImagePlaygroundDisabled == *request.Disabled {
		common.ApiSuccess(c, gin.H{"disabled": setting.ImagePlaygroundDisabled})
		return
	}
	setting.ImagePlaygroundDisabled = *request.Disabled
	if err := model.UpdateUserSetting(userId, setting); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"disabled": setting.ImagePlaygroundDisabled})
}
