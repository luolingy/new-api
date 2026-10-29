package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/tokenkit"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/tidwall/gjson"
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

	// contextKeyTokenTpmReserved stores how many tokens the admission check
	// already reserved in the current window, so settlement records only the
	// difference between the estimate and the real usage.
	contextKeyTokenTpmReserved = "token_tpm_reserved"
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

// tokenTpmAddScript 按 token 累计 TPM（INCRBY + 补写 EXPIRE）。
// ARGV[1] 允许为负数，用于结算时把预判多算的部分退回。
// 返回 {count, ttlSeconds}。
var tokenTpmAddScript = `
local count = redis.call('INCRBY', KEYS[1], ARGV[1])
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return {count, redis.call('TTL', KEYS[1])}
`

// tokenTpmReserveScript 在转发前原子地"预判 + 预留"本次请求的输入 token。
// 判断与累加在同一个脚本内完成，因此并发请求不会同时越过限制。
// KEYS[1] 计数键；ARGV[1] 预判 token；ARGV[2] 限制；ARGV[3] 窗口秒数。
// 返回 {allowed, ttlSeconds, current}。被拒时不修改计数。
var tokenTpmReserveScript = `
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
if current + tonumber(ARGV[1]) > tonumber(ARGV[2]) then
  local ttl = redis.call('TTL', KEYS[1])
  if ttl < 0 then
    ttl = tonumber(ARGV[3])
  end
  return {0, ttl, current}
end
local count = redis.call('INCRBY', KEYS[1], ARGV[1])
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[3])
end
return {1, redis.call('TTL', KEYS[1]), count}
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

// windowRetryAfter 返回当前固定窗口滚动前剩余的最少 1 秒等待时间。
func windowRetryAfter(now time.Time, win *tokenRateWindow) int64 {
	retryAfter := int64(tokenRateLimitWindowSeconds) - (now.Unix() - win.start)
	if retryAfter < 1 {
		retryAfter = 1
	}
	return retryAfter
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
		return false, windowRetryAfter(now, win)
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
		return false, windowRetryAfter(now, win)
	}
	return true, 0
}

// reserveTpm 原子地预判并预留本次请求的 token：判断与累加在同一把锁内完成，
// 因此并发请求不会都判定为"未超限"。被拒时不改动窗口计数。
func (l *tokenRateLimiter) reserveTpm(now time.Time, tokenId, tokens, limit int) (bool, int64) {
	if limit <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, tokenId)
	if win.tpm+tokens > limit {
		return false, windowRetryAfter(now, win)
	}
	win.tpm += tokens
	return true, 0
}

// recordTpm 在窗口内累计实际消耗 token（prompt+completion）。
// tokens 为负时用于退回预判多算的部分。
func (l *tokenRateLimiter) recordTpm(now time.Time, tokenId, tokens int) {
	if tokens == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, tokenId)
	win.tpm += tokens
	if win.tpm < 0 {
		win.tpm = 0
	}
}

var tokenRateLimitStore = newTokenRateLimiter(nil)

// ============================================================================
// 对外入口
// ============================================================================

// TokenRateLimitPass 在请求进入分发器时执行每 token 的 RPM/TPM 准入检查。
// 返回是否放行，以及被拒时的 Retry-After 秒数（放行时为 0）。
// tokenId 为 0 或两个限制均为 0 时直接放行（零开销）。
//
// estimatedTokens 是本次请求输入 token 的预判值。大于 0 时 TPM 检查改为
// 原子"预判 + 预留"：窗口已用量加上本次输入一旦超过限制就在转发前拒绝，
// 并且判断与累加在同一次原子操作内完成，并发请求不会同时误判为未超限。
// 预判值会在结算时按实际消耗补偿（见 RecordTokenUsage）。
func TokenRateLimitPass(c *gin.Context, tokenId, rpmLimit, tpmLimit, estimatedTokens int) (bool, int64) {
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
				return false, tokenRateLimitRetryAfter(retryAfter)
			}
		}
		if tpmLimit > 0 {
			var allowed bool
			var retryAfter int64
			var err error
			if estimatedTokens > 0 {
				allowed, retryAfter, err = tokenTpmRedisReserve(c, tokenId, estimatedTokens, tpmLimit)
				if err == nil && allowed {
					c.Set(contextKeyTokenTpmReserved, estimatedTokens)
				}
			} else {
				allowed, retryAfter, err = tokenTpmRedisPeek(c, tokenId, tpmLimit)
			}
			if err != nil {
				logger.LogError(context.Background(), fmt.Sprintf("token TPM check failed (tokenId=%d): %v", tokenId, err))
				return true, 0
			}
			if !allowed {
				return false, tokenRateLimitRetryAfter(retryAfter)
			}
		}
		return true, 0
	}

	now := tokenRateLimitStore.now()
	if rpmLimit > 0 {
		allowed, retryAfter := tokenRateLimitStore.takeRpm(now, tokenId, rpmLimit)
		if !allowed {
			return false, tokenRateLimitRetryAfter(retryAfter)
		}
	}
	if tpmLimit > 0 {
		if estimatedTokens > 0 {
			allowed, retryAfter := tokenRateLimitStore.reserveTpm(now, tokenId, estimatedTokens, tpmLimit)
			if !allowed {
				return false, tokenRateLimitRetryAfter(retryAfter)
			}
			c.Set(contextKeyTokenTpmReserved, estimatedTokens)
		} else {
			allowed, retryAfter := tokenRateLimitStore.peekTpm(now, tokenId, tpmLimit)
			if !allowed {
				return false, tokenRateLimitRetryAfter(retryAfter)
			}
		}
	}
	return true, 0
}

// EstimateRequestInputTokens estimates how many prompt tokens a request will
// consume from its raw JSON body, so the TPM admission check can reject an
// oversized request before it is forwarded. It reads the text fields the common
// relay protocols share and falls back to the whole body when none are present.
// The result is an estimate for admission control, never a billable count.
func EstimateRequestInputTokens(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	modelName := gjson.GetBytes(body, "model").String()
	var text strings.Builder
	appendText := func(value string) {
		if value == "" {
			return
		}
		text.WriteString(value)
		text.WriteString("\n")
	}
	if messages := gjson.GetBytes(body, "messages"); messages.IsArray() {
		for _, message := range messages.Array() {
			content := message.Get("content")
			if content.Type == gjson.String {
				appendText(content.String())
				continue
			}
			if content.IsArray() {
				for _, part := range content.Array() {
					appendText(part.Get("text").String())
				}
			}
		}
	}
	for _, path := range []string{"prompt", "system", "instructions", "input"} {
		if value := gjson.GetBytes(body, path); value.Type == gjson.String {
			appendText(value.String())
		}
	}
	if text.Len() == 0 {
		return tokenkit.Estimate(modelName, string(body))
	}
	return tokenkit.Estimate(modelName, text.String())
}

// tokenRateLimitRetryAfter 应用可配置的 429 持续时间。
// 配置为 0 时沿用窗口剩余时间；大于 0 时以配置值为准。
func tokenRateLimitRetryAfter(computed int64) int64 {
	if configured := setting.TokenRateLimitRetryAfterSeconds; configured > 0 {
		return int64(configured)
	}
	if computed < 1 {
		return 1
	}
	return computed
}

// RecordTokenUsage 在结算路径把实际消耗的 token（prompt+completion）
// 计入对应 token 的 TPM 窗口。限制为 0 或 tokenId 为 0 时不做任何事。
// 准入阶段已预留预判值的请求只补偿差额，避免同一批 token 被计算两次。
func RecordTokenUsage(c *gin.Context, tokenId, promptTokens, completionTokens int) {
	if tokenId <= 0 {
		return
	}
	tpmLimit := c.GetInt("token_tpm_limit")
	if tpmLimit <= 0 {
		return
	}
	actual := promptTokens + completionTokens
	reserved := c.GetInt(contextKeyTokenTpmReserved)
	delta := actual - reserved
	if delta == 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		recordTokenTpmRedis(tokenId, delta)
		return
	}
	tokenRateLimitStore.recordTpm(tokenRateLimitStore.now(), tokenId, delta)
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

// tokenTpmRedisReserve 在 Redis 中用单个 Lua 脚本完成 TPM 预判与预留，
// 保证并发请求不会都判定为未超限。被拒时计数保持不变。
func tokenTpmRedisReserve(c *gin.Context, tokenId, tokens, limit int) (bool, int64, error) {
	if common.RDB == nil {
		return false, 0, errors.New("Redis client is not initialized")
	}
	if limit <= 0 || tokens <= 0 {
		return true, 0, nil
	}
	values, err := common.RDB.Eval(
		context.Background(),
		tokenTpmReserveScript,
		[]string{fmt.Sprintf("%s:%d", tokenTpmRedisNamespace, tokenId)},
		tokens,
		limit,
		tokenRateLimitWindowSeconds,
	).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(values) != 3 {
		return false, 0, fmt.Errorf("unexpected Redis TPM reserve reply length %d", len(values))
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
