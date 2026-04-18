package controllerstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrLockNotAcquired = errors.New("failed to acquire lock")
	ErrLockExpired     = errors.New("lock has expired")
)

type HeartbeatStore struct {
	client *redis.Client
	ctx    context.Context
	cancel context.CancelFunc
	subs   map[string]chan<- *PubSubMessage
	mu     sync.RWMutex
}

type PubSubMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func NewHeartbeatStore(addr, password string, db int) (*HeartbeatStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		// Connection pool settings optimized for high concurrency
		PoolSize:        200,              // Maximum number of connections
		MinIdleConns:    20,               // Minimum idle connections to maintain
		MaxIdleConns:    50,               // Maximum idle connections
		ConnMaxIdleTime: 5 * time.Minute,  // Close connections idle for this long
		ConnMaxLifetime: 30 * time.Minute, // Maximum connection lifetime
		// Timeouts
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
		// Retry settings
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	if err := client.Ping(ctx).Err(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Use background context for long-lived operations
	bgCtx, bgCancel := context.WithCancel(context.Background())

	store := &HeartbeatStore{
		client: client,
		ctx:    bgCtx,
		cancel: bgCancel,
		subs:   make(map[string]chan<- *PubSubMessage),
	}

	// Clean up the ping context
	cancel()

	go store.handleSubscriptions()

	// Log connection pool stats
	stats := client.PoolStats()
	fmt.Printf("Redis connection pool initialized: poolSize=%d, hits=%d, misses=%d\n",
		stats.TotalConns, stats.Hits, stats.Misses)

	return store, nil
}

func (s *HeartbeatStore) Close() error {
	s.cancel()
	return s.client.Close()
}

func (s *HeartbeatStore) Client() *redis.Client {
	return s.client
}

func (s *HeartbeatStore) Ping() error {
	return s.client.Ping(s.ctx).Err()
}

func (s *HeartbeatStore) SetHeartbeat(publicKey string) error {
	key := fmt.Sprintf("aria:heartbeat:%s", publicKey)
	now := time.Now().Unix()
	pipe := s.client.Pipeline()
	pipe.Set(s.ctx, key, now, 2*time.Minute)
	pipe.HSet(s.ctx, "aria:heartbeats", publicKey, now)
	_, err := pipe.Exec(s.ctx)
	return err
}

func (s *HeartbeatStore) IsOnline(publicKey string) (bool, error) {
	key := fmt.Sprintf("aria:heartbeat:%s", publicKey)
	val, err := s.client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	lastSeen, _ := strconv.ParseInt(val, 10, 64)
	return time.Since(time.Unix(lastSeen, 0)) < 2*time.Minute, nil
}

func (s *HeartbeatStore) GetOnlineNodes() ([]string, error) {
	result, err := s.client.HGetAll(s.ctx, "aria:heartbeats").Result()
	if err != nil {
		return nil, err
	}

	var onlineNodes []string
	for publicKey, lastSeenStr := range result {
		lastSeen, _ := strconv.ParseInt(lastSeenStr, 10, 64)
		if time.Since(time.Unix(lastSeen, 0)) < 2*time.Minute {
			onlineNodes = append(onlineNodes, publicKey)
		}
	}

	return onlineNodes, nil
}

func (s *HeartbeatStore) GetHeartbeatTimestamp(publicKey string) (int64, error) {
	key := fmt.Sprintf("aria:heartbeat:%s", publicKey)
	val, err := s.client.Get(s.ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(val, 10, 64)
}

func (s *HeartbeatStore) RemoveHeartbeat(publicKey string) error {
	pipe := s.client.Pipeline()
	key := fmt.Sprintf("aria:heartbeat:%s", publicKey)
	pipe.Del(s.ctx, key)
	pipe.HDel(s.ctx, "aria:heartbeats", publicKey)
	_, err := pipe.Exec(s.ctx)
	return err
}

func (s *HeartbeatStore) PublishConfigChange(tenantID, configType string, data interface{}) error {
	channel := fmt.Sprintf("aria:config:%s", tenantID)
	msg := PubSubMessage{
		Type:    configType,
		Payload: json.RawMessage{},
	}
	if data != nil {
		payload, _ := json.Marshal(data)
		msg.Payload = payload
	}
	return s.client.Publish(s.ctx, channel, msg).Err()
}

func (s *HeartbeatStore) PublishNodeChange(publicKey string, action string) error {
	channel := "aria:nodes"
	msg := map[string]string{
		"public_key": publicKey,
		"action":     action,
		"timestamp":  fmt.Sprintf("%d", time.Now().Unix()),
	}
	return s.client.Publish(s.ctx, channel, msg).Err()
}

// PublishNodeDeleted 发布节点删除事件（阶段3）
func (s *HeartbeatStore) PublishNodeDeleted(publicKey, hostname, assignedIP string) error {
	channel := "aria:node:deleted"
	msg := map[string]string{
		"public_key":  publicKey,
		"hostname":    hostname,
		"assigned_ip": assignedIP,
		"timestamp":   fmt.Sprintf("%d", time.Now().Unix()),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return s.client.Publish(s.ctx, channel, data).Err()
}

func (s *HeartbeatStore) SubscribeConfigChange(tenantID string) <-chan *PubSubMessage {
	ch := make(chan *PubSubMessage, 10)
	s.mu.Lock()
	s.subs[tenantID] = ch
	s.mu.Unlock()

	pubsub := s.client.Subscribe(s.ctx, fmt.Sprintf("aria:config:%s", tenantID))
	go func() {
		for msg := range pubsub.Channel() {
			var pubMsg PubSubMessage
			if err := json.Unmarshal([]byte(msg.Payload), &pubMsg); err == nil {
				ch <- &pubMsg
			}
		}
		close(ch)
		s.mu.Lock()
		delete(s.subs, tenantID)
		s.mu.Unlock()
	}()

	return ch
}

func (s *HeartbeatStore) SubscribeAllNodes() <-chan map[string]string {
	ch := make(chan map[string]string, 10)

	pubsub := s.client.Subscribe(s.ctx, "aria:nodes")
	go func() {
		for msg := range pubsub.Channel() {
			var nodeMsg map[string]string
			if err := json.Unmarshal([]byte(msg.Payload), &nodeMsg); err == nil {
				ch <- nodeMsg
			}
		}
		close(ch)
	}()

	return ch
}

func (s *HeartbeatStore) handleSubscriptions() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.RLock()
			for _, ch := range s.subs {
				select {
				case ch <- &PubSubMessage{Type: "ping"}:
				default:
				}
			}
			s.mu.RUnlock()
		case <-s.ctx.Done():
			return
		}
	}
}

type DistributedLock struct {
	store    *HeartbeatStore
	key      string
	token    string
	ttl      time.Duration
	acquired bool
}

func (s *HeartbeatStore) AcquireLock(resource string, ttl time.Duration) (*DistributedLock, error) {
	return s.AcquireLockWithRetry(resource, ttl, 3, 100*time.Millisecond)
}

func (s *HeartbeatStore) AcquireLockWithRetry(resource string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (*DistributedLock, error) {
	key := fmt.Sprintf("aria:lock:%s", resource)
	token := fmt.Sprintf("%d", time.Now().UnixNano())

	for i := 0; i < maxRetries; i++ {
		acquired, err := s.client.SetNX(s.ctx, key, token, ttl).Result()
		if err != nil {
			return nil, fmt.Errorf("redis error: %v", err)
		}

		if acquired {
			return &DistributedLock{
				store:    s,
				key:      key,
				token:    token,
				ttl:      ttl,
				acquired: true,
			}, nil
		}

		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}

	return nil, ErrLockNotAcquired
}

func (l *DistributedLock) Release() error {
	if !l.acquired {
		return nil
	}

	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	_, err := l.store.client.Eval(l.store.ctx, script, []string{l.key}, l.token).Result()
	if err == nil {
		l.acquired = false
	}

	return err
}

func (l *DistributedLock) Extend(ttl time.Duration) error {
	if !l.acquired {
		return errors.New("lock not acquired")
	}

	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	newTTLMs := int64(ttl.Milliseconds())
	_, err := l.store.client.Eval(l.store.ctx, script, []string{l.key}, l.token, newTTLMs).Result()
	return err
}

func (l *DistributedLock) IsAcquired() bool {
	return l.acquired
}

type IPAMLock struct {
	*DistributedLock
}

func (s *HeartbeatStore) AcquireIPAMLock(tenantID string) (*IPAMLock, error) {
	resource := fmt.Sprintf("ipam:%s", tenantID)
	lock, err := s.AcquireLock(resource, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return &IPAMLock{
		DistributedLock: lock,
	}, nil
}

type RateLimiter struct {
	client *redis.Client
	ctx    context.Context
}

func NewRateLimiter(addr, password string, db int) (*RateLimiter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	return &RateLimiter{
		client: client,
		ctx:    context.Background(),
	}, nil
}

func (r *RateLimiter) Allow(key string, limit int, window time.Duration) (bool, error) {
	now := time.Now().UnixNano()
	windowStart := now - window.Nanoseconds()

	pipe := r.client.Pipeline()

	pipe.ZRemRangeByScore(r.ctx, key, "-inf", fmt.Sprintf("%d", windowStart))

	pipe.ZAdd(r.ctx, key, redis.Z{
		Score:  float64(now),
		Member: fmt.Sprintf("%d", now),
	})

	pipe.ZCard(r.ctx, key)

	pipe.Expire(r.ctx, key, window)

	cmds, err := pipe.Exec(r.ctx)
	if err != nil {
		return false, err
	}

	count := cmds[2].(*redis.IntCmd).Val()
	return count <= int64(limit), nil
}

func (r *RateLimiter) Close() error {
	return r.client.Close()
}
