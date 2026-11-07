package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// 每个渠道独立的 RPM（每分钟请求数）与 TPM（每分钟 token 消耗）限制，
// 语义与令牌级限制一致（见 token_rate_limit.go）：
//   - 0 表示不限制（默认），且完全跳过计数以避免开销。
//   - RPM：在渠道被选中后计数，窗口内请求数超过限制即拒绝。
//   - TPM：在请求结算时累计实际消耗的 prompt+completion token，
//     下一次请求到来时若窗口内累计已达到限制则拒绝。转发前会按输入
//     预判值做原子"预判 + 预留"，单个超出窗口的大请求会被放行，但窗口
//     内其后的请求会被拒绝，直到窗口滚动。
//
// 与令牌级限制的区别在于计数主体是渠道（channelId）而不是令牌，并且
// 因为渠道要在选中之后才知道，准入检查发生在分发器选中渠道之后。

const (
	channelRateLimitWindowSeconds = 60
	channelRpmRedisNamespace      = "rateLimit:v2:channel:rpm"
	channelTpmRedisNamespace      = "rateLimit:v2:channel:tpm"

	// contextKeyChannelTpmReserved stores how many tokens the admission check
	// already reserved in the current window, so settlement records only the
	// difference between the estimate and the real usage.
	contextKeyChannelTpmReserved = "channel_tpm_reserved"
)

// channelRpmTakeScript 按渠道计数 RPM（INCR + 首次 EXPIRE）。
// 返回 {allowed, ttlSeconds}。
var channelRpmTakeScript = `
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

// channelTpmAddScript 按渠道累计 TPM（INCRBY + 补写 EXPIRE）。
// ARGV[1] 允许为负数，用于结算时把预判多算的部分退回。
// 返回 {count, ttlSeconds}。
var channelTpmAddScript = `
local count = redis.call('INCRBY', KEYS[1], ARGV[1])
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return {count, redis.call('TTL', KEYS[1])}
`

// channelTpmReserveScript 在转发前原子地"预判 + 预留"本次请求的输入 token。
// 判断与累加在同一个脚本内完成，因此并发请求不会同时越过限制。
// KEYS[1] 计数键；ARGV[1] 预判 token；ARGV[2] 限制；ARGV[3] 窗口秒数。
// 返回 {allowed, ttlSeconds, current}。被拒时不修改计数。
var channelTpmReserveScript = `
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

// channelRateWindow 是内存模式下某个渠道的当前固定窗口。
type channelRateWindow struct {
	start int64 // 窗口起始 unix 秒
	rpm   int
	tpm   int
}

// channelRateLimiter 提供内存模式下的每渠道固定窗口计数。
// now 可注入以便测试。
type channelRateLimiter struct {
	mu      sync.Mutex
	windows map[int]*channelRateWindow
	now     func() time.Time
}

func newChannelRateLimiter(now func() time.Time) *channelRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &channelRateLimiter{windows: make(map[int]*channelRateWindow), now: now}
}

// activeWindow 返回 channelId 的当前窗口，窗口过期则重建。调用方需持锁。
func (l *channelRateLimiter) activeWindow(now time.Time, channelId int) *channelRateWindow {
	win, ok := l.windows[channelId]
	if !ok || now.Unix()-win.start >= channelRateLimitWindowSeconds {
		win = &channelRateWindow{start: now.Unix()}
		l.windows[channelId] = win
	}
	return win
}

// channelWindowRetryAfter 返回当前固定窗口滚动前剩余的最少 1 秒等待时间。
func channelWindowRetryAfter(now time.Time, win *channelRateWindow) int64 {
	retryAfter := int64(channelRateLimitWindowSeconds) - (now.Unix() - win.start)
	if retryAfter < 1 {
		retryAfter = 1
	}
	return retryAfter
}

// takeRpm 计数一次请求；limit<=0 表示不限制。返回是否允许与 Retry-After 秒数。
func (l *channelRateLimiter) takeRpm(now time.Time, channelId, limit int) (bool, int64) {
	if limit <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, channelId)
	win.rpm++
	if win.rpm > limit {
		return false, channelWindowRetryAfter(now, win)
	}
	return true, 0
}

// peekTpm 检查窗口内已累计 token 是否已达到限制；limit<=0 表示不限制。
func (l *channelRateLimiter) peekTpm(now time.Time, channelId, limit int) (bool, int64) {
	if limit <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, channelId)
	if win.tpm >= limit {
		return false, channelWindowRetryAfter(now, win)
	}
	return true, 0
}

// reserveTpm 原子地预判并预留本次请求的 token：判断与累加在同一把锁内完成，
// 因此并发请求不会都判定为"未超限"。被拒时不改动窗口计数。
func (l *channelRateLimiter) reserveTpm(now time.Time, channelId, tokens, limit int) (bool, int64) {
	if limit <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, channelId)
	if win.tpm+tokens > limit {
		return false, channelWindowRetryAfter(now, win)
	}
	win.tpm += tokens
	return true, 0
}

// recordTpm 在窗口内累计实际消耗 token（prompt+completion）。
// tokens 为负时用于退回预判多算的部分。
func (l *channelRateLimiter) recordTpm(now time.Time, channelId, tokens int) {
	if tokens == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	win := l.activeWindow(now, channelId)
	win.tpm += tokens
	if win.tpm < 0 {
		win.tpm = 0
	}
}

var channelRateLimitStore = newChannelRateLimiter(nil)

// ============================================================================
// 对外入口
// ============================================================================

// ChannelRateLimitPass 在渠道被选中后执行每渠道的 RPM/TPM 准入检查。
// 返回是否放行，以及被拒时的 Retry-After 秒数（放行时为 0）。
// channelId 为 0 或两个限制均为 0 时直接放行（零开销）。
//
// estimatedTokens 是本次请求输入 token 的预判值。大于 0 时 TPM 检查改为
// 原子"预判 + 预留"：窗口已用量加上本次输入一旦超过限制就在转发前拒绝，
// 并且判断与累加在同一次原子操作内完成，并发请求不会同时误判为未超限。
// 预判值会在结算时按实际消耗补偿（见 RecordChannelUsage）。
func ChannelRateLimitPass(c *gin.Context, channelId, rpmLimit, tpmLimit, estimatedTokens int) (bool, int64) {
	if channelId <= 0 || (rpmLimit <= 0 && tpmLimit <= 0) {
		return true, 0
	}
	if common.RedisEnabled && common.RDB != nil {
		if rpmLimit > 0 {
			allowed, retryAfter, err := channelRpmRedisTake(channelId, rpmLimit)
			if err != nil {
				logger.LogError(context.Background(), fmt.Sprintf("channel RPM check failed (channelId=%d): %v", channelId, err))
				return true, 0
			}
			if !allowed {
				return false, channelRateLimitRetryAfter(retryAfter)
			}
		}
		if tpmLimit > 0 {
			var allowed bool
			var retryAfter int64
			var err error
			if estimatedTokens > 0 {
				allowed, retryAfter, err = channelTpmRedisReserve(channelId, estimatedTokens, tpmLimit)
				if err == nil && allowed {
					c.Set(contextKeyChannelTpmReserved, estimatedTokens)
				}
			} else {
				allowed, retryAfter, err = channelTpmRedisPeek(channelId, tpmLimit)
			}
			if err != nil {
				logger.LogError(context.Background(), fmt.Sprintf("channel TPM check failed (channelId=%d): %v", channelId, err))
				return true, 0
			}
			if !allowed {
				return false, channelRateLimitRetryAfter(retryAfter)
			}
		}
		return true, 0
	}

	now := channelRateLimitStore.now()
	if rpmLimit > 0 {
		allowed, retryAfter := channelRateLimitStore.takeRpm(now, channelId, rpmLimit)
		if !allowed {
			return false, channelRateLimitRetryAfter(retryAfter)
		}
	}
	if tpmLimit > 0 {
		if estimatedTokens > 0 {
			allowed, retryAfter := channelRateLimitStore.reserveTpm(now, channelId, estimatedTokens, tpmLimit)
			if !allowed {
				return false, channelRateLimitRetryAfter(retryAfter)
			}
			c.Set(contextKeyChannelTpmReserved, estimatedTokens)
		} else {
			allowed, retryAfter := channelRateLimitStore.peekTpm(now, channelId, tpmLimit)
			if !allowed {
				return false, channelRateLimitRetryAfter(retryAfter)
			}
		}
	}
	return true, 0
}

// channelRateLimitRetryAfter 复用令牌级限制的 429 持续时间配置。
// 配置为 0 时沿用窗口剩余时间；大于 0 时以配置值为准。
func channelRateLimitRetryAfter(computed int64) int64 {
	if configured := setting.TokenRateLimitRetryAfterSeconds; configured > 0 {
		return int64(configured)
	}
	if computed < 1 {
		return 1
	}
	return computed
}

// RecordChannelUsage 在结算路径把实际消耗的 token（prompt+completion）
// 计入对应渠道的 TPM 窗口。限制为 0 或 channelId 为 0 时不做任何事。
// 准入阶段已预留预判值的请求只补偿差额，避免同一批 token 被计算两次。
func RecordChannelUsage(c *gin.Context, channelId, promptTokens, completionTokens int) {
	if channelId <= 0 {
		return
	}
	tpmLimit := c.GetInt("channel_tpm_limit")
	if tpmLimit <= 0 {
		return
	}
	actual := promptTokens + completionTokens
	reserved := c.GetInt(contextKeyChannelTpmReserved)
	delta := actual - reserved
	if delta == 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		recordChannelTpmRedis(channelId, delta)
		return
	}
	channelRateLimitStore.recordTpm(channelRateLimitStore.now(), channelId, delta)
}

// ============================================================================
// Redis 实现
// ============================================================================

func channelRpmRedisTake(channelId, limit int) (bool, int64, error) {
	if common.RDB == nil {
		return false, 0, errors.New("Redis client is not initialized")
	}
	if limit <= 0 {
		return true, 0, nil
	}
	values, err := common.RDB.Eval(
		context.Background(),
		channelRpmTakeScript,
		[]string{fmt.Sprintf("%s:%d", channelRpmRedisNamespace, channelId)},
		limit,
		channelRateLimitWindowSeconds,
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
		ttl = channelRateLimitWindowSeconds
	}
	return allowed == 1, ttl, nil
}

func channelTpmRedisPeek(channelId, limit int) (bool, int64, error) {
	if common.RDB == nil {
		return false, 0, errors.New("Redis client is not initialized")
	}
	if limit <= 0 {
		return true, 0, nil
	}
	key := fmt.Sprintf("%s:%d", channelTpmRedisNamespace, channelId)
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
			ttl = channelRateLimitWindowSeconds * time.Second
		}
		return false, int64(ttl / time.Second), nil
	}
	return true, 0, nil
}

// channelTpmRedisReserve 在 Redis 中用单个 Lua 脚本完成 TPM 预判与预留，
// 保证并发请求不会都判定为未超限。被拒时计数保持不变。
func channelTpmRedisReserve(channelId, tokens, limit int) (bool, int64, error) {
	if common.RDB == nil {
		return false, 0, errors.New("Redis client is not initialized")
	}
	if limit <= 0 || tokens <= 0 {
		return true, 0, nil
	}
	values, err := common.RDB.Eval(
		context.Background(),
		channelTpmReserveScript,
		[]string{fmt.Sprintf("%s:%d", channelTpmRedisNamespace, channelId)},
		tokens,
		limit,
		channelRateLimitWindowSeconds,
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
		ttl = channelRateLimitWindowSeconds
	}
	return allowed == 1, ttl, nil
}

func recordChannelTpmRedis(channelId, tokens int) {
	if common.RDB == nil || tokens <= 0 {
		return
	}
	_, err := common.RDB.Eval(
		context.Background(),
		channelTpmAddScript,
		[]string{fmt.Sprintf("%s:%d", channelTpmRedisNamespace, channelId)},
		tokens,
		channelRateLimitWindowSeconds,
	).Slice()
	if err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("failed to record channel TPM usage (channelId=%d): %v", channelId, err))
	}
}
