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
	assert.JSONEq(t, `{"success":false,"message":"The Classic frontend has been removed. The theme can only be set to default","message_key":"The Classic frontend has been removed. The theme can only be set to default"}`, response.Body.String())
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
		"theme.background_image":    "https://cdn.example.com/glass.png",
		"theme.background_position": "left top",
		"theme.glass_cards":         "true",
		"theme.card_opacity":        "42",
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"theme.background_image":    "",
			"theme.background_position": "",
			"theme.glass_cards":         "true",
			"theme.card_opacity":        "0",
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
				BackgroundImage    string `json:"background_image"`
				BackgroundPosition string `json:"background_position"`
				GlassCards         bool   `json:"glass_cards"`
				CardOpacity        int    `json:"card_opacity"`
			} `json:"site_theme"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.True(t, payload.Success)
	assert.Equal(t, "https://cdn.example.com/glass.png", payload.Data.SiteTheme.BackgroundImage)
	assert.Equal(t, "left top", payload.Data.SiteTheme.BackgroundPosition)
	assert.True(t, payload.Data.SiteTheme.GlassCards)
	assert.Equal(t, 42, payload.Data.SiteTheme.CardOpacity)
}

func TestValidateThemeCardOpacity(t *testing.T) {
	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "empty ok", value: "", valid: true},
		{name: "zero ok", value: "0", valid: true},
		{name: "mid", value: "55", valid: true},
		{name: "hundred", value: "100", valid: true},
		{name: "over", value: "101", valid: false},
		{name: "negative", value: "-1", valid: false},
		{name: "non numeric", value: "abc", valid: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := theme_setting.ValidateCardOpacity(testCase.value)
			if testCase.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateThemeBackgroundPosition(t *testing.T) {
	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "empty falls back to center", value: "", valid: true},
		{name: "center", value: "center", valid: true},
		{name: "left top", value: "left top", valid: true},
		{name: "case and space normalized", value: "  Right Bottom ", valid: true},
		{name: "arbitrary css rejected", value: "url(evil)", valid: false},
		{name: "percentage rejected", value: "20% 80%", valid: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := theme_setting.ValidateBackgroundPosition(testCase.value)
			if testCase.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
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
