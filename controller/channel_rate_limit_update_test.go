package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateChannelCanClearRateLimitsToZero is a regression test for the bug
// where setting a channel's rpm_limit/tpm_limit back to 0 appeared to save but
// the previous non-zero limits kept taking effect. The root cause was that
// channel.Update() reloads the row into memory (First) before the explicit
// column write, so reading channel.RpmLimit yielded the old DB value instead of
// the submitted 0. The fix reads the submitted numbers from the raw request body.
func TestUpdateChannelCanClearRateLimitsToZero(t *testing.T) {
	setupTaskPluginBindChannelTest(t)

	baseURL := "https://api.openai.com"
	channel := model.Channel{
		Type:               constant.ChannelTypeOpenAI,
		Status:             common.ChannelStatusEnabled,
		Name:               "rl-channel",
		Models:             "gpt-4o",
		Group:              "default",
		Key:                "sk-test",
		BaseURL:            &baseURL,
		RpmLimit:           120,
		TpmLimit:           200000,
	}
	require.NoError(t, channel.Insert())

	update := func(rpm, tpm int) string {
		return fmt.Sprintf(`{"id":%d,"type":1,"name":"rl-channel","key":"sk-test","models":"gpt-4o","group":"default","base_url":"https://api.openai.com","rpm_limit":%d,"tpm_limit":%d}`, channel.Id, rpm, tpm)
	}

	rec := putUpdateChannel(t, 1, common.RoleRootUser, update(0, 0))
	assert.Contains(t, rec.Body.String(), `"success":true`)

	var reloaded model.Channel
	require.NoError(t, model.DB.First(&reloaded, "id = ?", channel.Id).Error)
	assert.Equal(t, 0, reloaded.RpmLimit, "rpm_limit should be cleared to 0")
	assert.Equal(t, 0, reloaded.TpmLimit, "tpm_limit should be cleared to 0")

	// And a subsequent non-zero update must still apply.
	rec = putUpdateChannel(t, 1, common.RoleRootUser, update(60, 50000))
	assert.Contains(t, rec.Body.String(), `"success":true`)
	require.NoError(t, model.DB.First(&reloaded, "id = ?", channel.Id).Error)
	assert.Equal(t, 60, reloaded.RpmLimit)
	assert.Equal(t, 50000, reloaded.TpmLimit)
}
