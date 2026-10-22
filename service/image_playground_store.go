package service

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Offline image playground tasks are stored on disk so they survive a client
// disconnect and a server restart. Each task keeps its own directory:
//
//	<base>/users/<user id>/<task id>/task.json
//	<base>/users/<user id>/<task id>/ref-<n>.<ext>   (reference images)
//	<base>/users/<user id>/<task id>/out-<n>.<ext>   (generated images)
const (
	ImagePlaygroundStatusQueued    = "queued"
	ImagePlaygroundStatusRunning   = "running"
	ImagePlaygroundStatusSucceeded = "succeeded"
	ImagePlaygroundStatusFailed    = "failed"

	imagePlaygroundTaskFileName   = "task.json"
	imagePlaygroundMaxListedTasks = 100
)

// ImagePlaygroundAsset describes one generated image file.
type ImagePlaygroundAsset struct {
	File          string `json:"file"`
	MimeType      string `json:"mime_type"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImagePlaygroundPartial is the most recent progressive preview of a running
// task, produced by an upstream streaming response.
type ImagePlaygroundPartial struct {
	File      string `json:"file"`
	MimeType  string `json:"mime_type"`
	Index     int    `json:"index"`
	UpdatedAt int64  `json:"updated_at"`
}

// ImagePlaygroundTask is the durable record of one offline image generation.
type ImagePlaygroundTask struct {
	Id             string                  `json:"id"`
	UserId         int                     `json:"user_id"`
	Status         string                  `json:"status"`
	Prompt         string                  `json:"prompt"`
	Model          string                  `json:"model"`
	Group          string                  `json:"group"`
	Size           string                  `json:"size"`
	Quality        string                  `json:"quality"`
	Background     string                  `json:"background,omitempty"`
	Moderation     string                  `json:"moderation,omitempty"`
	OutputFormat   string                  `json:"output_format"`
	N              int                     `json:"n"`
	Stream         bool                    `json:"stream,omitempty"`
	PartialImages  int                     `json:"partial_images,omitempty"`
	ResponseFormat string                  `json:"response_format,omitempty"`
	ReferenceFiles []string                `json:"reference_files,omitempty"`
	Assets         []ImagePlaygroundAsset  `json:"assets,omitempty"`
	Partial        *ImagePlaygroundPartial `json:"partial,omitempty"`
	Error          string                  `json:"error,omitempty"`
	CreatedAt      int64                   `json:"created_at"`
	StartedAt      int64                   `json:"started_at,omitempty"`
	FinishedAt     int64                   `json:"finished_at,omitempty"`
}

var imagePlaygroundStore = struct {
	sync.RWMutex
	tasks  map[string]*ImagePlaygroundTask
	loaded bool
}{
	tasks: map[string]*ImagePlaygroundTask{},
}

// ImagePlaygroundBaseDir returns the directory that keeps every playground
// task. It can be overridden with IMAGE_PLAYGROUND_DIR.
func ImagePlaygroundBaseDir() string {
	if dir := strings.TrimSpace(os.Getenv("IMAGE_PLAYGROUND_DIR")); dir != "" {
		return dir
	}
	return filepath.Join("data", "image-playground")
}

func imagePlaygroundTaskDir(task *ImagePlaygroundTask) string {
	return filepath.Join(ImagePlaygroundBaseDir(), "users", fmt.Sprintf("%d", task.UserId), task.Id)
}

func imagePlaygroundTaskPath(task *ImagePlaygroundTask) string {
	return filepath.Join(imagePlaygroundTaskDir(task), imagePlaygroundTaskFileName)
}

// LoadImagePlaygroundTasks restores tasks from disk. It must run once before
// the worker starts; loading twice is a no-op.
func LoadImagePlaygroundTasks() error {
	imagePlaygroundStore.Lock()
	defer imagePlaygroundStore.Unlock()
	if imagePlaygroundStore.loaded {
		return nil
	}
	imagePlaygroundStore.loaded = true
	usersDir := filepath.Join(ImagePlaygroundBaseDir(), "users")
	userEntries, err := os.ReadDir(usersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, userEntry := range userEntries {
		if !userEntry.IsDir() {
			continue
		}
		taskEntries, err := os.ReadDir(filepath.Join(usersDir, userEntry.Name()))
		if err != nil {
			continue
		}
		for _, taskEntry := range taskEntries {
			if !taskEntry.IsDir() {
				continue
			}
			task := &ImagePlaygroundTask{}
			data, err := os.ReadFile(filepath.Join(usersDir, userEntry.Name(), taskEntry.Name(), imagePlaygroundTaskFileName))
			if err != nil {
				continue
			}
			if err := common.Unmarshal(data, task); err != nil || task.Id == "" {
				continue
			}
			imagePlaygroundStore.tasks[task.Id] = task
		}
	}
	return nil
}

func saveImagePlaygroundTaskLocked(task *ImagePlaygroundTask) error {
	dir := imagePlaygroundTaskDir(task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := common.Marshal(task)
	if err != nil {
		return err
	}
	path := imagePlaygroundTaskPath(task)
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

// CreateImagePlaygroundTask persists a new task record.
func CreateImagePlaygroundTask(task *ImagePlaygroundTask) error {
	imagePlaygroundStore.Lock()
	defer imagePlaygroundStore.Unlock()
	if err := saveImagePlaygroundTaskLocked(task); err != nil {
		return err
	}
	stored := *task
	imagePlaygroundStore.tasks[task.Id] = &stored
	return nil
}

// SaveImagePlaygroundTask overwrites the stored record of an existing task. A
// task deleted while it was running is not resurrected.
func SaveImagePlaygroundTask(task *ImagePlaygroundTask) error {
	imagePlaygroundStore.Lock()
	defer imagePlaygroundStore.Unlock()
	if _, ok := imagePlaygroundStore.tasks[task.Id]; !ok {
		return nil
	}
	if err := saveImagePlaygroundTaskLocked(task); err != nil {
		return err
	}
	stored := *task
	imagePlaygroundStore.tasks[task.Id] = &stored
	return nil
}

// ClaimImagePlaygroundTask moves a queued task to the running state. It
// returns false when the task is missing or already claimed.
func ClaimImagePlaygroundTask(taskId string) (*ImagePlaygroundTask, bool) {
	imagePlaygroundStore.Lock()
	defer imagePlaygroundStore.Unlock()
	task, ok := imagePlaygroundStore.tasks[taskId]
	if !ok || task.Status != ImagePlaygroundStatusQueued {
		return nil, false
	}
	task.Status = ImagePlaygroundStatusRunning
	task.StartedAt = time.Now().Unix()
	if err := saveImagePlaygroundTaskLocked(task); err != nil {
		common.SysError("failed to save image playground task: " + err.Error())
	}
	copied := *task
	return &copied, true
}

// GetImagePlaygroundTask returns a copy of the task owned by userId.
func GetImagePlaygroundTask(userId int, taskId string) (*ImagePlaygroundTask, bool) {
	imagePlaygroundStore.RLock()
	defer imagePlaygroundStore.RUnlock()
	task, ok := imagePlaygroundStore.tasks[taskId]
	if !ok || task.UserId != userId {
		return nil, false
	}
	copied := *task
	return &copied, true
}

// ListImagePlaygroundTasks returns the newest tasks of one user.
func ListImagePlaygroundTasks(userId int, limit int) []*ImagePlaygroundTask {
	if limit <= 0 || limit > imagePlaygroundMaxListedTasks {
		limit = imagePlaygroundMaxListedTasks
	}
	imagePlaygroundStore.RLock()
	tasks := make([]*ImagePlaygroundTask, 0, limit)
	for _, task := range imagePlaygroundStore.tasks {
		if task.UserId != userId {
			continue
		}
		copied := *task
		tasks = append(tasks, &copied)
	}
	imagePlaygroundStore.RUnlock()
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt == tasks[j].CreatedAt {
			return tasks[i].Id > tasks[j].Id
		}
		return tasks[i].CreatedAt > tasks[j].CreatedAt
	})
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	return tasks
}

// CountActiveImagePlaygroundTasks counts the user's queued or running tasks.
func CountActiveImagePlaygroundTasks(userId int) int {
	imagePlaygroundStore.RLock()
	defer imagePlaygroundStore.RUnlock()
	count := 0
	for _, task := range imagePlaygroundStore.tasks {
		if task.UserId != userId {
			continue
		}
		if task.Status == ImagePlaygroundStatusQueued || task.Status == ImagePlaygroundStatusRunning {
			count++
		}
	}
	return count
}

// DeleteImagePlaygroundTask removes the task record and every stored file.
func DeleteImagePlaygroundTask(userId int, taskId string) bool {
	imagePlaygroundStore.Lock()
	task, ok := imagePlaygroundStore.tasks[taskId]
	if !ok || task.UserId != userId {
		imagePlaygroundStore.Unlock()
		return false
	}
	delete(imagePlaygroundStore.tasks, taskId)
	imagePlaygroundStore.Unlock()
	_ = os.RemoveAll(imagePlaygroundTaskDir(task))
	return true
}

// WriteImagePlaygroundReference stores one uploaded reference image and
// returns its file name inside the task directory.
func WriteImagePlaygroundReference(task *ImagePlaygroundTask, index int, extension string, data []byte) (string, error) {
	dir := imagePlaygroundTaskDir(task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("ref-%d%s", index, extension)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return "", err
	}
	return name, nil
}

// ReadImagePlaygroundReference returns the bytes of a stored reference image.
func ReadImagePlaygroundReference(task *ImagePlaygroundTask, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(imagePlaygroundTaskDir(task), filepath.Base(name)))
}

// StoreImagePlaygroundAsset writes a generated image and returns its asset
// record. The extension always matches the sniffed content type.
func StoreImagePlaygroundAsset(task *ImagePlaygroundTask, index int, data []byte, revisedPrompt string) (ImagePlaygroundAsset, error) {
	mimeType := detectImageMimeType(data)
	dir := imagePlaygroundTaskDir(task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ImagePlaygroundAsset{}, err
	}
	name := fmt.Sprintf("out-%d%s", index, imageMimeExtension(mimeType))
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return ImagePlaygroundAsset{}, err
	}
	return ImagePlaygroundAsset{File: name, MimeType: mimeType, RevisedPrompt: revisedPrompt}, nil
}

// ReadImagePlaygroundAsset returns the bytes and content type of a stored image.
func ReadImagePlaygroundAsset(task *ImagePlaygroundTask, index int) ([]byte, string, error) {
	if index < 0 || index >= len(task.Assets) {
		return nil, "", fmt.Errorf("图片不存在")
	}
	asset := task.Assets[index]
	data, err := os.ReadFile(filepath.Join(imagePlaygroundTaskDir(task), filepath.Base(asset.File)))
	if err != nil {
		return nil, "", err
	}
	return data, asset.MimeType, nil
}

// StoreImagePlaygroundPartial replaces the progressive preview of a running
// task. Only the newest preview is kept, so the directory never accumulates
// partial frames.
func StoreImagePlaygroundPartial(task *ImagePlaygroundTask, index int, data []byte) (ImagePlaygroundPartial, error) {
	mimeType := detectImageMimeType(data)
	dir := imagePlaygroundTaskDir(task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ImagePlaygroundPartial{}, err
	}
	name := fmt.Sprintf("partial-%d%s", index, imageMimeExtension(mimeType))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return ImagePlaygroundPartial{}, err
	}
	if previous := task.Partial; previous != nil && previous.File != name {
		_ = os.Remove(filepath.Join(dir, filepath.Base(previous.File)))
	}
	return ImagePlaygroundPartial{
		File:      name,
		MimeType:  mimeType,
		Index:     index,
		UpdatedAt: time.Now().UnixMilli(),
	}, nil
}

// ReadImagePlaygroundPartial returns the newest progressive preview bytes.
func ReadImagePlaygroundPartial(task *ImagePlaygroundTask) ([]byte, string, error) {
	partial := task.Partial
	if partial == nil {
		return nil, "", fmt.Errorf("预览不存在")
	}
	path := filepath.Join(imagePlaygroundTaskDir(task), filepath.Base(partial.File))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, partial.MimeType, nil
}

// RemoveImagePlaygroundPartial deletes progressive previews once the final
// images are available.
func RemoveImagePlaygroundPartial(task *ImagePlaygroundTask) {
	if task.Partial == nil {
		return
	}
	path := filepath.Join(imagePlaygroundTaskDir(task), filepath.Base(task.Partial.File))
	_ = os.Remove(path)
	task.Partial = nil
}

// RemoveImagePlaygroundReferences deletes the uploaded reference images once a
// task has finished; generated images are kept for the retention period.
func RemoveImagePlaygroundReferences(task *ImagePlaygroundTask) {
	dir := imagePlaygroundTaskDir(task)
	for _, name := range task.ReferenceFiles {
		_ = os.Remove(filepath.Join(dir, filepath.Base(name)))
	}
	task.ReferenceFiles = nil
}

// RecoverImagePlaygroundTasks marks interrupted tasks as failed and returns the
// queued task ids that should be processed after a restart.
func RecoverImagePlaygroundTasks() []string {
	imagePlaygroundStore.Lock()
	pending := make([]string, 0, 8)
	interrupted := make([]*ImagePlaygroundTask, 0, 4)
	for _, task := range imagePlaygroundStore.tasks {
		switch task.Status {
		case ImagePlaygroundStatusRunning:
			task.Status = ImagePlaygroundStatusFailed
			task.Error = "服务器重启，任务已中断，请重新提交"
			task.FinishedAt = time.Now().Unix()
			copied := *task
			interrupted = append(interrupted, &copied)
		case ImagePlaygroundStatusQueued:
			pending = append(pending, task.Id)
		}
	}
	for _, task := range interrupted {
		_ = saveImagePlaygroundTaskLocked(task)
	}
	imagePlaygroundStore.Unlock()
	return pending
}

// PruneImagePlaygroundTasks deletes task directories older than retentionDays.
// A zero retention keeps tasks until the user removes them.
func PruneImagePlaygroundTasks(retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	imagePlaygroundStore.Lock()
	expired := make([]*ImagePlaygroundTask, 0, 8)
	for id, task := range imagePlaygroundStore.tasks {
		if task.Status == ImagePlaygroundStatusQueued || task.Status == ImagePlaygroundStatusRunning {
			continue
		}
		if task.CreatedAt > cutoff {
			continue
		}
		delete(imagePlaygroundStore.tasks, id)
		expired = append(expired, task)
	}
	imagePlaygroundStore.Unlock()
	for _, task := range expired {
		_ = os.RemoveAll(imagePlaygroundTaskDir(task))
	}
}

func detectImageMimeType(data []byte) string {
	contentType := strings.ToLower(http.DetectContentType(data))
	switch contentType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return contentType
	default:
		return "image/png"
	}
}

func imageMimeExtension(mimeType string) string {
	switch mimeType {
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
