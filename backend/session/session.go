package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const CookieName = "session"

type Data struct {
	UserID    uint64 `json:"user_id"`
	UserUUID  string `json:"user_uuid"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

func key(token string) string         { return "session:" + token }
func userSetKey(userID uint64) string { return "user_sessions:" + strconv.FormatUint(userID, 10) }

func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Create stores a new session in Redis and enforces SESSION_MAX_CONCURRENT by
// evicting the oldest session for the user when the limit is exceeded.
func Create(ctx context.Context, rdb *redis.Client, token string, data Data, ttl time.Duration, maxConcurrent int) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	pipe := rdb.TxPipeline()
	pipe.Set(ctx, key(token), b, ttl)
	pipe.ZAdd(ctx, userSetKey(data.UserID), redis.Z{Score: float64(time.Now().Unix()), Member: token})
	pipe.Expire(ctx, userSetKey(data.UserID), ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	// Evict oldest sessions beyond the concurrent limit.
	count, err := rdb.ZCard(ctx, userSetKey(data.UserID)).Result()
	if err == nil && int(count) > maxConcurrent {
		toEvict := int(count) - maxConcurrent
		oldest, err := rdb.ZRange(ctx, userSetKey(data.UserID), 0, int64(toEvict-1)).Result()
		if err == nil {
			for _, t := range oldest {
				rdb.Del(ctx, key(t))
				rdb.ZRem(ctx, userSetKey(data.UserID), t)
			}
		}
	}
	return nil
}

func Get(ctx context.Context, rdb *redis.Client, token string) (*Data, error) {
	b, err := rdb.Get(ctx, key(token)).Bytes()
	if err != nil {
		return nil, err
	}
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func Delete(ctx context.Context, rdb *redis.Client, token string, userID uint64) error {
	pipe := rdb.TxPipeline()
	pipe.Del(ctx, key(token))
	pipe.ZRem(ctx, userSetKey(userID), token)
	_, err := pipe.Exec(ctx)
	return err
}

// DeleteAllForUser force-logs-out every session belonging to a user.
func DeleteAllForUser(ctx context.Context, rdb *redis.Client, userID uint64) error {
	tokens, err := rdb.ZRange(ctx, userSetKey(userID), 0, -1).Result()
	if err != nil {
		return err
	}
	for _, t := range tokens {
		rdb.Del(ctx, key(t))
	}
	return rdb.Del(ctx, userSetKey(userID)).Err()
}
