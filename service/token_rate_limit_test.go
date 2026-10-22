package service

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

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
		allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, rpmLimit, 0)
		assert.True(t, allowed, "request %d should be allowed", i)
		assert.Zero(t, retryAfter)
	}

	// The next request within the same window is rejected with a retry hint.
	allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, rpmLimit, 0)
	assert.False(t, allowed)
	assert.Positive(t, retryAfter)

	// The window still rejects until it rolls over.
	clock.advance(30 * time.Second)
	allowed, _ = TokenRateLimitPass(ctx, tokenId, rpmLimit, 0)
	assert.False(t, allowed)

	// After the window rolls over, counting starts fresh.
	clock.advance(31 * time.Second)
	for i := 0; i < rpmLimit; i++ {
		allowed, _ := TokenRateLimitPass(ctx, tokenId, rpmLimit, 0)
		assert.True(t, allowed, "request %d after rollover should be allowed", i)
	}
	allowed, retryAfter = TokenRateLimitPass(ctx, tokenId, rpmLimit, 0)
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
	allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, 0, 1000)
	assert.True(t, allowed)
	assert.Zero(t, retryAfter)

	// Record usage: 600 tokens, still below 1000.
	RecordTokenUsage(ctx, tokenId, 600, 0)
	allowed, _ = TokenRateLimitPass(ctx, tokenId, 0, 1000)
	assert.True(t, allowed)

	// Record 400 more: total 1000, which is at the limit, so rejected.
	RecordTokenUsage(ctx, tokenId, 400, 0)
	allowed, retryAfter = TokenRateLimitPass(ctx, tokenId, 0, 1000)
	assert.False(t, allowed)
	assert.Positive(t, retryAfter)

	// Window rolls over: usage resets, admitted again.
	clock.advance(61 * time.Second)
	allowed, _ = TokenRateLimitPass(ctx, tokenId, 0, 1000)
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
		allowed, retryAfter := TokenRateLimitPass(ctx, tokenId, 0, 0)
		require.True(t, allowed)
		assert.Zero(t, retryAfter)
	}

	// Recording usage is a no-op when the token has no TPM limit.
	ctx.Set("token_tpm_limit", 0)
	RecordTokenUsage(ctx, tokenId, 500, 500)
	allowed, _ := TokenRateLimitPass(ctx, tokenId, 0, 0)
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
		allowed, retryAfter := TokenRateLimitPass(ctx, 0, 1, 1)
		assert.True(t, allowed)
		assert.Zero(t, retryAfter)
	}
	RecordTokenUsage(ctx, 0, 100, 100)
	allowed, _ := TokenRateLimitPass(ctx, 0, 1, 1)
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
	allowed, _ := TokenRateLimitPass(ctx, tokenId, 0, 100)
	assert.True(t, allowed)
}
