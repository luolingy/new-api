package service

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// fakeChannelClock is a manually advanced clock for deterministic window tests.
type fakeChannelClock struct {
	mu   sync.Mutex
	nows time.Time
}

func (f *fakeChannelClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nows
}

func (f *fakeChannelClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nows = f.nows.Add(d)
}

// installChannelRateLimitStore swaps the package store with a test store and
// restores it afterwards, so exported entry points run against a controlled
// clock and empty state.
func installChannelRateLimitStore(t *testing.T, clock *fakeChannelClock) {
	t.Helper()
	original := channelRateLimitStore
	channelRateLimitStore = newChannelRateLimiter(clock.now)
	t.Cleanup(func() { channelRateLimitStore = original })
}

func disableChannelRateLimitRedis(t *testing.T) {
	t.Helper()
	original := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = original })
}

func TestChannelRateLimitRpmAdmissionAndRollover(t *testing.T) {
	clock := &fakeChannelClock{nows: time.Unix(1_700_000_000, 0)}
	installChannelRateLimitStore(t, clock)
	disableChannelRateLimitRedis(t)

	ctx := &gin.Context{}
	const channelId = 7
	const rpmLimit = 3

	for i := 0; i < rpmLimit; i++ {
		allowed, retryAfter := ChannelRateLimitPass(ctx, channelId, rpmLimit, 0, 0)
		assert.True(t, allowed, "request %d should be allowed", i)
		assert.Zero(t, retryAfter)
	}

	allowed, retryAfter := ChannelRateLimitPass(ctx, channelId, rpmLimit, 0, 0)
	assert.False(t, allowed, "request over the limit should be rejected")
	assert.Equal(t, int64(channelRateLimitWindowSeconds), retryAfter)

	// A different channel has its own independent window.
	allowed, _ = ChannelRateLimitPass(ctx, channelId+1, rpmLimit, 0, 0)
	assert.True(t, allowed)

	// Rolling the window forward admits requests again.
	clock.advance(time.Duration(channelRateLimitWindowSeconds) * time.Second)
	allowed, retryAfter = ChannelRateLimitPass(ctx, channelId, rpmLimit, 0, 0)
	assert.True(t, allowed)
	assert.Zero(t, retryAfter)
}

func TestChannelRateLimitZeroLimitSkipsCounting(t *testing.T) {
	clock := &fakeChannelClock{nows: time.Unix(1_700_000_000, 0)}
	installChannelRateLimitStore(t, clock)
	disableChannelRateLimitRedis(t)

	ctx := &gin.Context{}
	for i := 0; i < 100; i++ {
		allowed, retryAfter := ChannelRateLimitPass(ctx, 0, 1, 1, 0)
		assert.True(t, allowed)
		assert.Zero(t, retryAfter)
	}
	// channelId 0 means no channel selected; nothing is counted or blocked.
	for i := 0; i < 100; i++ {
		allowed, _ := ChannelRateLimitPass(ctx, 5, 0, 0, 0)
		assert.True(t, allowed)
	}
}

func TestChannelRateLimitTpmRecordsAndBlocks(t *testing.T) {
	clock := &fakeChannelClock{nows: time.Unix(1_700_000_000, 0)}
	installChannelRateLimitStore(t, clock)
	disableChannelRateLimitRedis(t)

	ctx := &gin.Context{}
	const channelId = 9
	const tpmLimit = 1000
	ctx.Set("channel_tpm_limit", tpmLimit)

	allowed, _ := ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 0)
	assert.True(t, allowed)

	RecordChannelUsage(ctx, channelId, 600, 0)
	allowed, _ = ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 0)
	assert.True(t, allowed)

	RecordChannelUsage(ctx, channelId, 400, 0)
	allowed, retryAfter := ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 0)
	assert.False(t, allowed, "reaching the TPM limit should block the next request")
	assert.Equal(t, int64(channelRateLimitWindowSeconds), retryAfter)

	clock.advance(time.Duration(channelRateLimitWindowSeconds) * time.Second)
	allowed, _ = ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 0)
	assert.True(t, allowed, "rolling the window should admit requests again")
}

func TestChannelRateLimitTpmReserveCompensatesSettlement(t *testing.T) {
	clock := &fakeChannelClock{nows: time.Unix(1_700_000_000, 0)}
	installChannelRateLimitStore(t, clock)
	disableChannelRateLimitRedis(t)

	ctx := &gin.Context{}
	const channelId = 11
	const tpmLimit = 1000
	ctx.Set("channel_tpm_limit", tpmLimit)

	// Admission reserves an estimated 600 input tokens.
	allowed, _ := ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 600)
	assert.True(t, allowed)

	// Settlement records actual 600+100=700 total; only the 100 delta is added,
	// proving the 600 reserved at admission is not double counted.
	RecordChannelUsage(ctx, channelId, 600, 100)

	// Window now holds 700. A 500-token request would exceed 1000 (700+500), so
	// it is rejected before forwarding; the rejection does not consume the window.
	allowed, _ = ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 500)
	assert.False(t, allowed, "700 used + 500 reserved exceeds the 1000 limit")

	// A 300-token request fits exactly within the remaining 300 budget.
	allowed, _ = ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 300)
	assert.True(t, allowed, "reserve should be compensated to 700, leaving 300 available")

	// The window is now full (700+300=1000); any more is blocked.
	allowed, _ = ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 1)
	assert.False(t, allowed)
}

func TestChannelRateLimitReserveRejectsOversizedBeforeForward(t *testing.T) {
	clock := &fakeChannelClock{nows: time.Unix(1_700_000_000, 0)}
	installChannelRateLimitStore(t, clock)
	disableChannelRateLimitRedis(t)

	ctx := &gin.Context{}
	const channelId = 13
	const tpmLimit = 100

	// Estimated 500 tokens exceeds a 100 limit immediately, without counting.
	allowed, retryAfter := ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 500)
	assert.False(t, allowed)
	assert.Equal(t, int64(channelRateLimitWindowSeconds), retryAfter)

	// The rejected request did not consume the window.
	allowed, _ = ChannelRateLimitPass(ctx, channelId, 0, tpmLimit, 50)
	assert.True(t, allowed)
}
