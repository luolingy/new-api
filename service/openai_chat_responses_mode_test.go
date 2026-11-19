package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatResponsesChannelOverride(t *testing.T) {
	global := model_setting.GetGlobalSettings()
	previous := global.ChatCompletionsToResponsesPolicy
	t.Cleanup(func() { global.ChatCompletionsToResponsesPolicy = previous })
	for _, tc := range []struct {
		name          string
		setting       string
		globalEnabled bool
		model         string
		want          bool
	}{
		{name: "enabled without global policy", setting: `{"chat_completions_to_responses_enabled":true}`, model: "other-model", want: true},
		{name: "disabled overrides matching global policy", setting: `{"chat_completions_to_responses_enabled":false}`, globalEnabled: true, model: "gpt-test"},
		{name: "unset inherits matching global policy", setting: `{}`, globalEnabled: true, model: "gpt-test", want: true},
		{name: "unset preserves model filtering", setting: `{}`, globalEnabled: true, model: "other-model"},
		{name: "unset preserves global disabled", setting: `{}`, model: "gpt-test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			global.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: tc.globalEnabled, AllChannels: true, ModelPatterns: []string{"^gpt-test$"}}
			var settings dto.ChannelSettings
			require.NoError(t, common.UnmarshalJsonStr(tc.setting, &settings))
			assert.Equal(t, tc.want, ShouldChatCompletionsUseResponses(settings.ChatCompletionsToResponsesEnabled, 1, 1, tc.model))
			encoded, err := common.Marshal(settings)
			require.NoError(t, err)
			var restored dto.ChannelSettings
			require.NoError(t, common.Unmarshal(encoded, &restored))
			assert.Equal(t, settings.ChatCompletionsToResponsesEnabled, restored.ChatCompletionsToResponsesEnabled, "explicit false and absent must remain distinct after saving")
		})
	}
}
