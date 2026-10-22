package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// 每个令牌独立的 RPM（每分钟请求数）与 TPM（每分钟 token 消耗）限制。
// 语义：
//   - 0 表示不限制（默认），且完全跳过计数以避免开销。
//   - RPM：在请求进入分发器时计数，窗口内请求数超过限制即拒绝。
//   - TPM：在请求结算时累计实际消耗的 prompt+completion token，
//     下一次请求到来时若窗口内累计已达到限制则拒绝。单个超出窗口的
//     大请求会被放行，但窗口内其后的请求会被拒绝，直到窗口滚动。

const (
	tokenRateLimitWindowSeconds = 60
	tokenRpmRedisNamespace      = "rateLimit:v2:token:rpm"
	tokenTpmRedisNamespace      = "rateLimit:v2:token:tpm"
)

// tokenRpmTakeScript 按 token 计数 RPM（INCR + 首次 EXPIRE）。
// 返回 {allowed, ttlSeconds}。
var tokenRpmTakeScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
  ttl = redis.call('TTL', KEYS[1])
end
if count > tonumber(ARGV[1]) then
  return {0, ttl}
end
return {1, ttl}
`

// tokenTpmAddScript 按 token 累计 TPM（INCRBY + 首次 EXPIRE）。
// 返回 {count, ttlSeconds}。
var tokenTpmAddScript = `
local count = redis.call('INCRBY', KEYS[1], ARGV[1])
if count == tonumber(ARGV[1]) then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
  ttl = redis.call('TTL', KEYS[1])
end
return {count, ttl}
`

// tokenRateWindow 是内存模式下某个 token 的当前固定窗口。
type tokenRateWindow struct {
	start int64 // 窗口起始 unix 秒
	rpm   int
	tpm   int
}

// tokenRateLimiter 提供内存模式下的每 token 固定窗口计数。
// now 可注入以便测试。
type tokenRateLimiter struct {
	mu      sync.Mutex
	windows map[int]*tokenRateWindow
	now     func() time.Time
}

func newTokenRateLimiter(now func() time.Time) *tokenRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &tokenRateLimiter{windows: make(map[int]*tokenRateWindow), now: now}
}

// activeWindow 返回 tokenId 的当前窗口，窗口过期则重建。调用方需持锁。
func (l *tokenRateLimiter) activeWindow(now time.Time, tokenId int) *tokenRateWindow {
	win, ok := l.windows[tokenId]
	if !ok || now.Unix()-win.start >= tokenRateLimitWindowSeconds {
		win = &tokenRateWindow{start: now.Unix()}
		l.windows[tokenId] = win
	}
	return win
}

// takeRpm 计数一次请求；limit<=0 表示不限制。返回是否允许与 Retry-After 秒数。
func (l *tokenRateLimiter) takeRpm(now time.Time, tokenId, limit int) (bool, int64) {
	if limit <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, tokenId)
	win.rpm++
	if win.rpm > limit {
		retryAfter := int64(tokenRateLimitWindowSeconds) - (now.Unix() - win.start)
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}
	return true, 0
}

// peekTpm 检查窗口内已累计 token 是否已达到限制；limit<=0 表示不限制。
func (l *tokenRateLimiter) peekTpm(now time.Time, tokenId, limit int) (bool, int64) {
	if limit <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, tokenId)
	if win.tpm >= limit {
		retryAfter := int64(tokenRateLimitWindowSeconds) - (now.Unix() - win.start)
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}
	return true, 0
}

// recordTpm 在窗口内累计实际消耗 token（prompt+completion）。
func (l *tokenRateLimiter) recordTpm(now time.Time, tokenId, tokens int) {
	if tokens <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, tokenId)
	win.tpm += tokens
}

var tokenRateLimitStore = newTokenRateLimiter(nil)

// ============================================================================
// 对外入口
// ============================================================================

// TokenRateLimitPass 在请求进入分发器时执行每 token 的 RPM/TPM 准入检查。
// 返回是否放行，以及被拒时的 Retry-After 秒数（放行时为 0）。
// tokenId 为 0 或两个限制均为 0 时直接放行（零开销）。
func TokenRateLimitPass(c *gin.Context, tokenId, rpmLimit, tpmLimit int) (bool, int64) {
	if tokenId <= 0 || (rpmLimit <= 0 && tpmLimit <= 0) {
		return true, 0
	}
	if common.RedisEnabled && common.RDB != nil {
		if rpmLimit > 0 {
			allowed, retryAfter, err := tokenRpmRedisTake(c, tokenId, rpmLimit)
			if err != nil {
				logger.LogError(context.Background(), fmt.Sprintf("token RPM check failed (tokenId=%d): %v", tokenId, err))
				return true, 0
			}
			if !allowed {
				return false, retryAfter
			}
		}
		if tpmLimit > 0 {
			allowed, retryAfter, err := tokenTpmRedisPeek(c, tokenId, tpmLimit)
			if err != nil {
				logger.LogError(context.Background(), fmt.Sprintf("token TPM check failed (tokenId=%d): %v", tokenId, err))
				return true, 0
			}
			if !allowed {
				return false, retryAfter
			}
		}
		return true, 0
	}

	now := tokenRateLimitStore.now()
	if rpmLimit > 0 {
		allowed, retryAfter := tokenRateLimitStore.takeRpm(now, tokenId, rpmLimit)
		if !allowed {
			return false, retryAfter
		}
	}
	if tpmLimit > 0 {
		allowed, retryAfter := tokenRateLimitStore.peekTpm(now, tokenId, tpmLimit)
		if !allowed {
			return false, retryAfter
		}
	}
	return true, 0
}

// RecordTokenUsage 在结算路径把实际消耗的 token（prompt+completion）
// 计入对应 token 的 TPM 窗口。限制为 0 或 tokenId 为 0 时不做任何事。
func RecordTokenUsage(c *gin.Context, tokenId, promptTokens, completionTokens int) {
	if tokenId <= 0 {
		return
	}
	tpmLimit := c.GetInt("token_tpm_limit")
	if tpmLimit <= 0 {
		return
	}
	tokens := promptTokens + completionTokens
	if tokens <= 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		recordTokenTpmRedis(tokenId, tokens)
		return
	}
	tokenRateLimitStore.recordTpm(tokenRateLimitStore.now(), tokenId, tokens)
}

// ============================================================================
// Redis 实现
// ============================================================================

func tokenRpmRedisTake(c *gin.Context, tokenId, limit int) (bool, int64, error) {
	if common.RDB == nil {
		return false, 0, errors.New("Redis client is not initialized")
	}
	if limit <= 0 {
		return true, 0, nil
	}
	values, err := common.RDB.Eval(
		context.Background(),
		tokenRpmTakeScript,
		[]string{fmt.Sprintf("%s:%d", tokenRpmRedisNamespace, tokenId)},
		limit,
		tokenRateLimitWindowSeconds,
	).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(values) != 2 {
		return false, 0, fmt.Errorf("unexpected Redis RPM reply length %d", len(values))
	}
	allowed, err := redisReplyIntegerValue(values[0])
	if err != nil {
		return false, 0, err
	}
	ttl, err := redisReplyIntegerValue(values[1])
	if err != nil {
		return false, 0, err
	}
	if ttl <= 0 {
		ttl = tokenRateLimitWindowSeconds
	}
	return allowed == 1, ttl, nil
}

func tokenTpmRedisPeek(c *gin.Context, tokenId, limit int) (bool, int64, error) {
	if common.RDB == nil {
		return false, 0, errors.New("Redis client is not initialized")
	}
	if limit <= 0 {
		return true, 0, nil
	}
	key := fmt.Sprintf("%s:%d", tokenTpmRedisNamespace, tokenId)
	count, err := common.RDB.Get(context.Background(), key).Int()
	if err != nil && errors.Is(err, redis.Nil) {
		return true, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if count >= limit {
		ttl, ttlErr := common.RDB.TTL(context.Background(), key).Result()
		if ttlErr != nil || ttl <= 0 {
			ttl = tokenRateLimitWindowSeconds * time.Second
		}
		return false, int64(ttl / time.Second), nil
	}
	return true, 0, nil
}

func recordTokenTpmRedis(tokenId, tokens int) {
	if common.RDB == nil || tokens <= 0 {
		return
	}
	_, err := common.RDB.Eval(
		context.Background(),
		tokenTpmAddScript,
		[]string{fmt.Sprintf("%s:%d", tokenTpmRedisNamespace, tokenId)},
		tokens,
		tokenRateLimitWindowSeconds,
	).Slice()
	if err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("failed to record token TPM usage (tokenId=%d): %v", tokenId, err))
	}
}

func redisReplyIntegerValue(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis integer reply type %T", value)
	}
}
