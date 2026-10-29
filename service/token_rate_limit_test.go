package service

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTokenClock is a manually advanced clock for deterministic window tests.
type fakeTokenClock struct {
	mu   sync.Mutex
	nows time.Time
}

func (f *fakeTokenClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nows
}

func (f *fakeTokenClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nows = f.nows.Add(d)
}

// installTokenRateLimitStore swaps the package store with a test store and
// restores it afterwards, so exported entry points run against a controlled
// clock and empty state.
func installTokenRateLimitStore(t *testing.T, clock *fakeTokenClock) {
	t.Helper()
	original := tokenRateLimitStore
	store := newTokenRateLimiter(clock.now)
	tokenRateLimitStore = store
	t.Cleanup(func() { tokenRateLimitStore = original })
}

func TestTokenRateLimitRpmAdmissionAndRollover(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	ctx := &gin.Context{}
	const tokenId = 7
	const rpmLimit = 3

	// The first rpmLimit requests are allowed.
	for i := 0; i < rpmLimit; i++ {
		allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, rpmLimit, 0, 0)
		assert.True(t, allowed, "request %d should be allowed", i)
		assert.Zero(t, retryAfter)
	}

	// The next request within the same window is rejected with a retry hint.
	allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, rpmLimit, 0, 0)
	assert.False(t, allowed)
	assert.Positive(t, retryAfter)

	// The window still rejects until it rolls over.
	clock.advance(30 * time.Second)
	allowed, _ = TokenRateLimitPass(ctx, tokenId, rpmLimit, 0, 0)
	assert.False(t, allowed)

	// After the window rolls over, counting starts fresh.
	clock.advance(31 * time.Second)
	for i := 0; i < rpmLimit; i++ {
		allowed, _ := TokenRateLimitPass(ctx, tokenId, rpmLimit, 0, 0)
		assert.True(t, allowed, "request %d after rollover should be allowed", i)
	}
	allowed, retryAfter = TokenRateLimitPass(ctx, tokenId, rpmLimit, 0, 0)
	assert.False(t, allowed)
	assert.Positive(t, retryAfter)
}

func TestTokenRateLimitTpmAccumulationAdmission(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	ctx := &gin.Context{}
	ctx.Set("token_tpm_limit", 1000)
	const tokenId = 9

	// Below the limit: admitted.
	allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, 0, 1000, 0)
	assert.True(t, allowed)
	assert.Zero(t, retryAfter)

	// Record usage: 600 tokens, still below 1000.
	RecordTokenUsage(ctx, tokenId, 600, 0)
	allowed, _ = TokenRateLimitPass(ctx, tokenId, 0, 1000, 0)
	assert.True(t, allowed)

	// Record 400 more: total 1000, which is at the limit, so rejected.
	RecordTokenUsage(ctx, tokenId, 400, 0)
	allowed, retryAfter = TokenRateLimitPass(ctx, tokenId, 0, 1000, 0)
	assert.False(t, allowed)
	assert.Positive(t, retryAfter)

	// Window rolls over: usage resets, admitted again.
	clock.advance(61 * time.Second)
	allowed, _ = TokenRateLimitPass(ctx, tokenId, 0, 1000, 0)
	assert.True(t, allowed)
}

func TestTokenRateLimitZeroMeansUnlimited(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	ctx := &gin.Context{}
	const tokenId = 11

	// A zero limit accepts everything without consuming anything.
	for i := 0; i < 100; i++ {
		allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, 0, 0, 0)
		require.True(t, allowed)
		assert.Zero(t, retryAfter)
	}

	// Recording usage is a no-op when the token has no TPM limit.
	ctx.Set("token_tpm_limit", 0)
	RecordTokenUsage(ctx, tokenId, 500, 500)
	allowed, _ := TokenRateLimitPass(ctx, tokenId, 0, 0, 0)
	assert.True(t, allowed)
}

func TestTokenRateLimitSkipsZeroTokenId(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	// tokenId == 0 (playground / user auth paths) bypasses limits entirely.
	ctx := &gin.Context{}
	for i := 0; i < 10; i++ {
		allowed, retryAfter := TokenRateLimitPass(ctx, 0, 1, 1, 0)
		assert.True(t, allowed)
		assert.Zero(t, retryAfter)
	}
	RecordTokenUsage(ctx, 0, 100, 100)
	allowed, _ := TokenRateLimitPass(ctx, 0, 1, 1, 0)
	assert.True(t, allowed)
}

func TestTokenRateLimitRecordTpmIgnoresZeroOrNegativeTokens(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	ctx := &gin.Context{}
	ctx.Set("token_tpm_limit", 100)
	const tokenId = 13

	// Zero/negative usage must not corrupt the window.
	RecordTokenUsage(ctx, tokenId, 0, 0)
	RecordTokenUsage(ctx, tokenId, 0, -5)
	allowed, _ := TokenRateLimitPass(ctx, tokenId, 0, 100, 0)
	assert.True(t, allowed)
}

func TestTokenRateLimitRejectsBeforeForwardingWhenInputExceedsLimit(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	ctx := &gin.Context{}
	ctx.Set("token_tpm_limit", 1000)
	const tokenId = 21

	// The window is still empty, but the request input alone already exceeds the
	// limit, so it must be rejected before forwarding.
	allowed, _ := TokenRateLimitPass(ctx, tokenId, 0, 1000, 1200)
	assert.False(t, allowed)

	// A request that fits is admitted and its estimate is reserved.
	allowed, _ = TokenRateLimitPass(ctx, tokenId, 0, 1000, 600)
	assert.True(t, allowed)
	assert.Equal(t, 600, ctx.GetInt(contextKeyTokenTpmReserved))

	// Accumulated reservation plus the next input now exceeds the limit.
	allowed, _ = TokenRateLimitPass(ctx, tokenId, 0, 1000, 500)
	assert.False(t, allowed)
}

func TestTokenRateLimitReserveIsAtomicUnderConcurrency(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	const (
		tokenId    = 22
		limit      = 1000
		perRequest = 400
		callers    = 8
	)
	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := 0
	for range callers {
		wg.Go(func() {
			ctx := &gin.Context{}
			ctx.Set("token_tpm_limit", limit)
			allowed, _ := TokenRateLimitPass(ctx, tokenId, 0, limit, perRequest)
			if allowed {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	// Only two 400-token requests fit under a 1000-token window; concurrent
	// callers must never be admitted on a stale read.
	assert.Equal(t, 2, admitted)
}

func TestTokenRateLimitSettlementReconcilesReservation(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedis })

	ctx := &gin.Context{}
	ctx.Set("token_tpm_limit", 1000)
	const tokenId = 23

	allowed, _ := TokenRateLimitPass(ctx, tokenId, 0, 1000, 600)
	require.True(t, allowed)

	// Settlement records only the difference, so the window keeps the real usage
	// instead of the estimate plus the usage.
	RecordTokenUsage(ctx, tokenId, 200, 100)
	assert.Equal(t, 300, tokenRateLimitStore.windows[tokenId].tpm)

	// The reclaimed headroom is usable again.
	allowed, _ = TokenRateLimitPass(ctx, tokenId, 0, 1000, 700)
	assert.True(t, allowed)
}

func TestTokenRateLimitUsesConfiguredRetryAfter(t *testing.T) {
	clock := &fakeTokenClock{nows: time.Unix(1_700_000_000, 0)}
	installTokenRateLimitStore(t, clock)
	originalRedis := common.RedisEnabled
	common.RedisEnabled = false
	originalRetryAfter := setting.TokenRateLimitRetryAfterSeconds
	t.Cleanup(func() {
		common.RedisEnabled = originalRedis
		setting.TokenRateLimitRetryAfterSeconds = originalRetryAfter
	})

	ctx := &gin.Context{}
	ctx.Set("token_tpm_limit", 100)
	const tokenId = 24

	setting.TokenRateLimitRetryAfterSeconds = 0
	allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, 0, 100, 500)
	require.False(t, allowed)
	assert.GreaterOrEqual(t, retryAfter, int64(1))
	assert.LessOrEqual(t, retryAfter, int64(tokenRateLimitWindowSeconds))

	setting.TokenRateLimitRetryAfterSeconds = 30
	allowed, retryAfter = TokenRateLimitPass(ctx, tokenId, 0, 100, 500)
	require.False(t, allowed)
	assert.Equal(t, int64(30), retryAfter)
}

func TestEstimateRequestInputTokensReadsCommonTextFields(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello world"},{"role":"user","content":[{"type":"text","text":"second part"}]}]}`)
	estimated := EstimateRequestInputTokens(body)
	assert.Greater(t, estimated, 0)

	assert.Equal(t, 0, EstimateRequestInputTokens(nil))

	// A body without the common text fields still yields a positive estimate.
	fallback := EstimateRequestInputTokens([]byte(`{"model":"gpt-4o","foo":"bar baz"}`))
	assert.Greater(t, fallback, 0)
}
