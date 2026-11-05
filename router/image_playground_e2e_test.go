package router

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// imagePlaygroundUpstream is an in-test OpenAI-compatible image endpoint. It
// records every request the gateway forwards, so a test can assert both that
// the offline pipeline produced images and that the caller's configuration
// actually reached the upstream.
type imagePlaygroundUpstream struct {
	mu       sync.Mutex
	requests []imagePlaygroundUpstreamRequest
	mode     string // b64 | url | stream | error
	delay    time.Duration
	images   map[string][]byte
}

type imagePlaygroundUpstreamRequest struct {
	Path           string
	Model          string
	Prompt         string
	N              string
	Size           string
	Quality        string
	Background     string
	Moderation     string
	OutputFormat   string
	ResponseFormat string
	Stream         string
	PartialImages  string
	ImageParts     int
	HasAuth        bool
}

func (u *imagePlaygroundUpstream) reset(mode string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests = nil
	u.mode = mode
	u.images = map[string][]byte{}
}

func (u *imagePlaygroundUpstream) lastRequest(t *testing.T) imagePlaygroundUpstreamRequest {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	require.NotEmpty(t, u.requests, "the gateway never called the upstream")
	return u.requests[len(u.requests)-1]
}

// decodeImagePlaygroundUpstreamRequest flattens JSON and multipart bodies into
// one key/value view and reports how many image parts (image[]/image) arrived.
func decodeImagePlaygroundUpstreamRequest(r *http.Request) (map[string]string, int) {
	values := map[string]string{}
	parts := 0
	if len(r.URL.Query()) > 0 {
		for key, list := range r.URL.Query() {
			if len(list) > 0 {
				values[key] = list[0]
			}
		}
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return values, 0
		}
		for key, list := range r.MultipartForm.Value {
			if len(list) > 0 {
				values[key] = list[0]
			}
		}
		for name, list := range r.MultipartForm.File {
			if name == "image" || name == "image[]" {
				parts += len(list)
			}
		}
		return values, parts
	}
	body := make([]byte, 0)
	buffer := bytes.NewBuffer(body)
	if _, err := buffer.ReadFrom(r.Body); err != nil {
		return values, 0
	}
	if buffer.Len() == 0 {
		return values, 0
	}
	var payload map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &payload); err != nil {
		return values, 0
	}
	for key, value := range payload {
		values[key] = fmt.Sprint(value)
	}
	return values, 0
}

// imagePlaygroundTestPNG returns a small, valid PNG whose pixels differ by index
// so stored assets can be told apart.
func imagePlaygroundTestPNG(index int) []byte {
	canvas := image.NewRGBA(image.Rect(0, 0, 16, 16))
	fill := color.NRGBA{R: uint8(30 + index*70), G: uint8(80 + index*40), B: uint8(200 - index*50), A: 255}
	for y := range 16 {
		for x := range 16 {
			if (x/4+y/4+index)%2 == 0 {
				canvas.Set(x, y, fill)
			} else {
				canvas.Set(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	var sink bytes.Buffer
	_ = png.Encode(&sink, canvas)
	return sink.Bytes()
}

func (u *imagePlaygroundUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/stub-images/") {
		u.serveImage(w, r)
		return
	}
	values, parts := decodeImagePlaygroundUpstreamRequest(r)
	record := imagePlaygroundUpstreamRequest{
		Path:           r.URL.Path,
		Model:          values["model"],
		Prompt:         values["prompt"],
		N:              values["n"],
		Size:           values["size"],
		Quality:        values["quality"],
		Background:     values["background"],
		Moderation:     values["moderation"],
		OutputFormat:   values["output_format"],
		ResponseFormat: values["response_format"],
		Stream:         values["stream"],
		PartialImages:  values["partial_images"],
		ImageParts:     parts,
		HasAuth:        len(r.Header.Get("Authorization")) > 0,
	}
	u.mu.Lock()
	u.requests = append(u.requests, record)
	mode := u.mode
	u.mu.Unlock()

	if u.delay > 0 {
		time.Sleep(u.delay)
	}
	if mode == "error" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"stub rejected the prompt on purpose"}}`))
		return
	}

	count := 1
	if values["n"] != "" {
		if parsed, err := strconv.Atoi(values["n"]); err == nil && parsed > 0 {
			count = parsed
		}
	}
	partials := 1
	if values["partial_images"] != "" {
		if parsed, err := strconv.Atoi(values["partial_images"]); err == nil && parsed >= 0 {
			partials = parsed
		}
	}
	wantsStream := mode == "stream" || values["stream"] == "true" || values["stream"] == "1"

	if wantsStream {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		write := func(payload any) {
			encoded, _ := json.Marshal(payload)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
			if flusher != nil {
				flusher.Flush()
			}
		}
		write(map[string]any{"type": "image_generation.created"})
		for index := range partials {
			write(map[string]any{
				"type":                "image_generation.partial_image",
				"partial_image_index": index,
				"b64_json":            base64.StdEncoding.EncodeToString(imagePlaygroundTestPNG(index)),
			})
		}
		for index := range count {
			write(map[string]any{
				"type":           "image_generation.completed",
				"b64_json":       base64.StdEncoding.EncodeToString(imagePlaygroundTestPNG(index)),
				"revised_prompt": fmt.Sprintf("stub stream image %d", index+1),
			})
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}

	items := make([]map[string]string, 0, count)
	for index := range count {
		data := imagePlaygroundTestPNG(index)
		item := map[string]string{"revised_prompt": fmt.Sprintf("stub image %d for %q", index+1, values["prompt"])}
		if mode == "url" {
			name := fmt.Sprintf("img-%d-%d.png", time.Now().UnixNano(), index)
			u.mu.Lock()
			u.images[name] = data
			u.mu.Unlock()
			scheme := "http"
			item["url"] = scheme + "://" + r.Host + "/stub-images/" + name
		} else {
			item["b64_json"] = base64.StdEncoding.EncodeToString(data)
		}
		items = append(items, item)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"created": time.Now().Unix(), "data": items})
}

func (u *imagePlaygroundUpstream) serveImage(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/stub-images/"):]
	u.mu.Lock()
	data, ok := u.images[name]
	u.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}

// imagePlaygroundE2E bundles the database, gateway listener and stub upstream
// one offline-generation test needs.
type imagePlaygroundE2E struct {
	upstream *imagePlaygroundUpstream
	userId   int
	model    string
}

func setupImagePlaygroundE2E(t *testing.T) *imagePlaygroundE2E {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init(), "the relay error path resolves messages through i18n")
	// The stream scanner derives its tickers from this global; main normally sets
	// it at startup, and a zero value panics inside time.NewTicker.
	previousStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousStreamingTimeout })

	previousDB := model.DB
	previousMain := common.MainDatabaseType()
	previousLog := common.LogDatabaseType()
	previousSQLitePath := common.SQLitePath
	previousMasterNode := common.IsMasterNode
	previousRedis := common.RedisEnabled
	previousMemoryCache := common.MemoryCacheEnabled

	common.SQLitePath = t.TempDir() + "/image-playground-e2e.db"
	common.IsMasterNode = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Setenv("SQL_DSN", "")
	require.NoError(t, model.InitDB())
	database := model.DB
	model.LOG_DB = database
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.UserSubscription{}))
	t.Cleanup(func() {
		if sqlDB, err := database.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.SQLitePath = previousSQLitePath
		common.IsMasterNode = previousMasterNode
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
	})

	const modelName = "gpt-image-1-stub"

	// Price per image keeps the per-call image billing path real: without a
	// configured price the relay rejects the request before reaching the upstream.
	previousPrices := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(fmt.Sprintf(`{%q: 0.04}`, modelName)))
	t.Cleanup(func() { _ = ratio_setting.UpdateModelPriceByJSONString(previousPrices) })

	upstream := &imagePlaygroundUpstream{images: map[string][]byte{}}
	upstream.reset("b64")
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)

	require.NoError(t, database.Create(&model.User{
		Username: "image-playground-e2e",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    10_000_000_000,
	}).Error)
	var user model.User
	require.NoError(t, database.Where("username = ?", "image-playground-e2e").First(&user).Error)

	baseURL := server.URL
	require.NoError(t, database.Create(&model.Channel{
		Id:      1,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "sk-stub",
		Status:  common.ChannelStatusEnabled,
		Name:    "image-playground-stub",
		Models:  modelName,
		Group:   "default",
		BaseURL: &baseURL,
	}).Error)
	channel := model.Channel{}
	require.NoError(t, database.First(&channel, 1).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetTaskPluginProtocolRouter(engine)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	t.Setenv("PORT", strconv.Itoa(port))
	t.Setenv("IMAGE_PLAYGROUND_DIR", t.TempDir())
	gateway := &http.Server{Handler: engine, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = gateway.Serve(listener) }()
	t.Cleanup(func() { _ = gateway.Close() })

	service.StartImagePlaygroundWorker()

	return &imagePlaygroundE2E{upstream: upstream, userId: user.Id, model: modelName}
}

// prepare fills the identity fields the store and worker derive paths from, so a
// caller can write reference images before the task is enqueued.
func (e *imagePlaygroundE2E) prepare(task *service.ImagePlaygroundTask) *service.ImagePlaygroundTask {
	task.UserId = e.userId
	task.Group = "default"
	task.Model = e.model
	task.Status = service.ImagePlaygroundStatusQueued
	if task.Id == "" {
		task.Id = fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	}
	if task.CreatedAt == 0 {
		task.CreatedAt = time.Now().Unix()
	}
	return task
}

// submit creates one task, enqueues it, and waits for it to finish.
func (e *imagePlaygroundE2E) submit(t *testing.T, task *service.ImagePlaygroundTask) *service.ImagePlaygroundTask {
	t.Helper()
	e.prepare(task)
	require.NoError(t, service.CreateImagePlaygroundTask(task))
	service.ImagePlaygroundEnqueue(task.Id)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		current, ok := service.GetImagePlaygroundTask(e.userId, task.Id)
		require.True(t, ok, "the task disappeared from the store")
		if current.Status == service.ImagePlaygroundStatusSucceeded || current.Status == service.ImagePlaygroundStatusFailed {
			return current
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach a terminal status within the deadline", task.Id)
	return nil
}

func assertPNGAsset(t *testing.T, task *service.ImagePlaygroundTask, index int) {
	t.Helper()
	data, mimeType, err := service.ReadImagePlaygroundAsset(task, index)
	require.NoError(t, err)
	assert.Equal(t, "image/png", mimeType)
	require.NotEmpty(t, data)
	assert.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, data[:4], "stored asset %d is not a PNG", index)
}

func TestImagePlaygroundOfflineGenerationPipeline(t *testing.T) {
	env := setupImagePlaygroundE2E(t)

	t.Run("json b64 single image", func(t *testing.T) {
		env.upstream.reset("b64")
		task := env.submit(t, &service.ImagePlaygroundTask{
			Prompt: "a cat", Size: "1024x1024", Quality: "high", OutputFormat: "png", N: 1,
		})
		require.Equal(t, service.ImagePlaygroundStatusSucceeded, task.Status, task.Error)
		require.Len(t, task.Assets, 1)
		assertPNGAsset(t, task, 0)
		assert.Contains(t, task.Assets[0].RevisedPrompt, "a cat")

		forwarded := env.upstream.lastRequest(t)
		assert.Equal(t, "/v1/images/generations", forwarded.Path)
		assert.Equal(t, "1024x1024", forwarded.Size)
		assert.Equal(t, "high", forwarded.Quality)
		assert.Equal(t, "png", forwarded.OutputFormat)
		assert.Equal(t, env.model, forwarded.Model)
		assert.True(t, forwarded.HasAuth)
	})

	t.Run("json b64 multiple images", func(t *testing.T) {
		env.upstream.reset("b64")
		task := env.submit(t, &service.ImagePlaygroundTask{
			Prompt: "two dogs", Size: "1536x1024", Quality: "medium", OutputFormat: "png", N: 3,
		})
		require.Equal(t, service.ImagePlaygroundStatusSucceeded, task.Status, task.Error)
		require.Len(t, task.Assets, 3)
		for index := range task.Assets {
			assertPNGAsset(t, task, index)
		}
		assert.Equal(t, "3", env.upstream.lastRequest(t).N)
	})

	t.Run("streaming partial previews", func(t *testing.T) {
		env.upstream.reset("stream")
		task := env.submit(t, &service.ImagePlaygroundTask{
			Prompt: "a streamed fox", Size: "1024x1024", Quality: "high", OutputFormat: "png",
			N: 2, Stream: true, PartialImages: 2,
		})
		require.Equal(t, service.ImagePlaygroundStatusSucceeded, task.Status, task.Error)
		require.Len(t, task.Assets, 2)
		for index := range task.Assets {
			assertPNGAsset(t, task, index)
		}
		forwarded := env.upstream.lastRequest(t)
		assert.Equal(t, "true", forwarded.Stream)
		assert.Equal(t, "2", forwarded.PartialImages)
	})

	t.Run("reference images use the edits endpoint", func(t *testing.T) {
		env.upstream.reset("b64")
		task := env.prepare(&service.ImagePlaygroundTask{
			Prompt: "blend these", Size: "1024x1024", Quality: "high", OutputFormat: "png", N: 1,
		})
		for index := range 2 {
			name, err := service.WriteImagePlaygroundReference(task, index, ".png", imagePlaygroundTestPNG(index))
			require.NoError(t, err)
			task.ReferenceFiles = append(task.ReferenceFiles, name)
		}
		finished := env.submit(t, task)
		require.Equal(t, service.ImagePlaygroundStatusSucceeded, finished.Status, finished.Error)
		require.Len(t, finished.Assets, 1)
		assertPNGAsset(t, finished, 0)

		forwarded := env.upstream.lastRequest(t)
		assert.Equal(t, "/v1/images/edits", forwarded.Path)
		assert.Equal(t, 2, forwarded.ImageParts)
	})

	t.Run("url responses are downloaded", func(t *testing.T) {
		env.upstream.reset("url")
		task := env.submit(t, &service.ImagePlaygroundTask{
			Prompt: "a downloaded otter", Size: "1024x1024", Quality: "high", OutputFormat: "png", N: 2,
		})
		require.Equal(t, service.ImagePlaygroundStatusSucceeded, task.Status, task.Error)
		require.Len(t, task.Assets, 2)
		for index := range task.Assets {
			assertPNGAsset(t, task, index)
		}
	})

	t.Run("upstream errors fail the task with the message", func(t *testing.T) {
		env.upstream.reset("error")
		task := env.submit(t, &service.ImagePlaygroundTask{
			Prompt: "will fail", Size: "1024x1024", Quality: "high", OutputFormat: "png", N: 1,
		})
		require.Equal(t, service.ImagePlaygroundStatusFailed, task.Status)
		assert.Contains(t, task.Error, "stub rejected the prompt on purpose")
	})
}
