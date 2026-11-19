package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		statusCode       int
		statusCodeConfig string
		expectedCode     int
	}{
		{
			name:             "map string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"503"}`,
			expectedCode:     503,
		},
		{
			name:             "map int value",
			statusCode:       429,
			statusCodeConfig: `{"429":503}`,
			expectedCode:     503,
		},
		{
			name:             "skip invalid string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"bad-code"}`,
			expectedCode:     429,
		},
		{
			name:             "skip status code 200",
			statusCode:       200,
			statusCodeConfig: `{"200":503}`,
			expectedCode:     200,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newAPIError := &types.NewAPIError{
				StatusCode: tc.statusCode,
			}
			ResetStatusCode(newAPIError, tc.statusCodeConfig)
			require.Equal(t, tc.expectedCode, newAPIError.StatusCode)
		})
	}
}

func TestRelayErrorClientMessageStatesErrorOrigin(t *testing.T) {
	t.Parallel()

	upstream := types.WithOpenAIError(types.OpenAIError{
		Message: "rate limit reached",
		Type:    "rate_limit_error",
		Code:    "rate_limit_exceeded",
	}, http.StatusTooManyRequests)
	local := types.NewErrorWithStatusCode(errors.New("model price not configured"), types.ErrorCodeModelPriceError, http.StatusBadRequest)

	for _, tc := range []struct {
		name         string
		err          *types.NewAPIError
		lang         string
		wantUpstream bool
		wantType     string
		wantMsg      string
	}{
		{name: "upstream error in Chinese", err: upstream, lang: "zh-CN", wantUpstream: true, wantType: "rate_limit_error", wantMsg: "上游错误：rate limit reached"},
		{name: "local error in Chinese", err: local, lang: "zh-CN", wantType: "new_api_error", wantMsg: "本站错误：model price not configured"},
		{name: "upstream error in English", err: upstream, lang: "en", wantUpstream: true, wantType: "rate_limit_error", wantMsg: "Upstream error: rate limit reached"},
		{name: "local error in English", err: local, lang: "en", wantType: "new_api_error", wantMsg: "Gateway error: model price not configured"},
		{name: "traditional Chinese", err: local, lang: "zh-TW", wantType: "new_api_error", wantMsg: "本站錯誤：model price not configured"},
		{name: "upstream transport failure", err: types.NewError(errors.New("connection failed"), types.ErrorCodeDoRequestFailed), lang: "en", wantUpstream: true, wantType: "new_api_error", wantMsg: "Upstream error: connection failed"},
		{name: "wrapped upstream failure", err: types.NewErrorWithStatusCode(upstream, types.ErrorCodeInvalidRequest, http.StatusBadRequest), lang: "en", wantUpstream: true, wantType: "new_api_error", wantMsg: "Upstream error: rate limit reached"},
		{name: "unclassified error is local", err: &types.NewAPIError{Err: errors.New("unknown failure")}, lang: "en", wantMsg: "Gateway error: unknown failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.wantUpstream, tc.err.IsUpstreamError())
			openAIErr := OpenAIErrorToClient(testContextWithLanguage(tc.lang), tc.err)
			assert.Equal(t, tc.wantMsg, openAIErr.Message, "the client must be able to tell where the error came from")
			assert.Equal(t, tc.wantType, openAIErr.Type, "the origin prefix must not change the upstream error type")
			assert.Equal(t, tc.wantMsg, ClaudeErrorToClient(testContextWithLanguage(tc.lang), tc.err).Message)
			assert.Equal(t, tc.err.ToOpenAIError().Code, openAIErr.Code)
			assert.Equal(t, strings.TrimPrefix(tc.wantMsg, ErrorOriginPrefix(testContextWithLanguage(tc.lang), tc.wantUpstream)), tc.err.Error(), "client rendering must not change stored error messages")
		})
	}
}

// testContextWithLanguage is a request context whose reader states lang, which
// is enough to resolve a backend message.
func testContextWithLanguage(lang string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Accept-Language", lang)
	return c
}

func TestRelayErrorHandlerTruncatesInvalidJSONBodyInLog(t *testing.T) {
	withDebugEnabled(t, false)

	body := strings.Repeat("b", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, "bad response status code 500", newAPIError.Error())
	require.Contains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), fmt.Sprintf("original_length=%d", len(body)))
	require.NotContains(t, logBuffer.String(), strings.Repeat("b", common.LocalLogContentLimit+1))
}

func TestRelayErrorHandlerKeepsStructuredErrorMessage(t *testing.T) {
	message := strings.Repeat("c", common.LocalLogContentLimit+256)
	body := `{"message":"` + message + `"}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
}

func TestRelayErrorHandlerKeepsOpenAIErrorMessage(t *testing.T) {
	message := strings.Repeat("d", common.LocalLogContentLimit+256)
	body := `{"error":{"message":"` + message + `","type":"server_error","code":"server_error"}}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
}

func TestRelayErrorHandlerKeepsInvalidJSONBodyInDebugLog(t *testing.T) {
	withDebugEnabled(t, true)

	body := strings.Repeat("e", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.NotContains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), body)
}

func withDebugEnabled(t *testing.T, enabled bool) {
	t.Helper()

	oldDebug := common.DebugEnabled
	common.DebugEnabled = enabled
	t.Cleanup(func() {
		common.DebugEnabled = oldDebug
	})
}
