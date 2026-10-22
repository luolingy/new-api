package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/theme_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionRejectsRetiredFrontendTheme(t *testing.T) {
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/",
		strings.NewReader(`{"key":"theme.frontend","value":"classic"}`),
	)

	UpdateOption(context)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"success":false,"message":"Classic 前端已移除，主题只能设置为 default"}`, response.Body.String())
}

func TestGetStatusAdvertisesDefaultDashboard(t *testing.T) {
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousMap })
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(context)

	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.True(t, payload.Success)
	assert.Equal(t, "default", payload.Data["theme"])
}

func TestGetStatusExposesSiteTheme(t *testing.T) {
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousMap })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"theme.background_image": "https://cdn.example.com/glass.png",
		"theme.glass_cards":      "true",
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"theme.background_image": "",
			"theme.glass_cards":      "true",
		}))
	})

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(context)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			SiteTheme struct {
				BackgroundImage string `json:"background_image"`
				GlassCards      bool   `json:"glass_cards"`
			} `json:"site_theme"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.True(t, payload.Success)
	assert.Equal(t, "https://cdn.example.com/glass.png", payload.Data.SiteTheme.BackgroundImage)
	assert.True(t, payload.Data.SiteTheme.GlassCards)
}

func TestValidateThemeBackgroundImageURL(t *testing.T) {
	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "empty disables the theme", value: "", valid: true},
		{name: "https URL", value: "https://cdn.example.com/bg.png", valid: true},
		{name: "http URL with query", value: "http://127.0.0.1:8080/bg.jpg?v=2", valid: true},
		{name: "root relative path", value: "/assets/bg.webp", valid: true},
		{name: "javascript scheme", value: "javascript:alert(1)", valid: false},
		{name: "data scheme", value: "data:image/png;base64,AAAA", valid: false},
		{name: "protocol relative", value: "//evil.example.com/bg.png", valid: false},
		{name: "quote injection", value: `https://cdn.example.com/a.png") , url(evil`, valid: false},
		{name: "unquoted parenthesis", value: "https://cdn.example.com/a(1).png", valid: false},
		{name: "whitespace", value: "https://cdn.example.com/a b.png", valid: false},
		{name: "missing host", value: "https:///bg.png", valid: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := theme_setting.ValidateBackgroundImageURL(testCase.value)
			if testCase.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
