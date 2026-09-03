package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRedisChannel = "flight:edge:task_changed:v1"

// RedisFanout combines local delivery with best-effort cross-replica delivery.
// Redis is never the source of truth: a publish failure is recoverable through
// the employee's next full task snapshot.
type RedisFanout struct {
	local   *InMemoryFanout
	client  *redis.Client
	channel string
	origin  string
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	close   sync.Once
}

type redisNotificationEnvelope struct {
	Origin                   string      `json:"origin"`
	AudienceEmployeePublicID string      `json:"audience_employee_public_id"`
	Notification             TaskChanged `json:"notification"`
}

func NewRedisFanout(local *InMemoryFanout, client *redis.Client, channel, origin string) (*RedisFanout, error) {
	if client == nil {
		return nil, errors.New("redis notification client is required")
	}
	if strings.TrimSpace(origin) == "" {
		return nil, errors.New("redis notification origin is required")
	}
	if local == nil {
		local = NewInMemoryFanout()
	}
	if strings.TrimSpace(channel) == "" {
		channel = defaultRedisChannel
	}
	ctx, cancel := context.WithCancel(context.Background())
	fanout := &RedisFanout{local: local, client: client, channel: channel, origin: origin, ctx: ctx, cancel: cancel}
	fanout.wg.Add(1)
	go fanout.subscribeLoop()
	return fanout, nil
}

func (fanout *RedisFanout) Subscribe(employeePublicID, connectionID string, sink Sink) (func(), error) {
	if fanout == nil || fanout.local == nil {
		return nil, ErrInvalidSubscription
	}
	return fanout.local.Subscribe(employeePublicID, connectionID, sink)
}

func (fanout *RedisFanout) Publish(ctx context.Context, notification TaskChanged) (PublishResult, error) {
	if fanout == nil || fanout.local == nil || fanout.client == nil {
		return PublishResult{}, errors.New("redis notification fan-out is not configured")
	}
	if ctx == nil {
		return PublishResult{}, errors.New("notification publish context is nil")
	}
	localResult, localErr := fanout.local.Publish(ctx, notification)
	envelope, marshalErr := json.Marshal(redisNotificationEnvelope{Origin: fanout.origin, AudienceEmployeePublicID: notification.AudienceEmployeePublicID, Notification: notification})
	if marshalErr != nil {
		return localResult, errors.Join(localErr, fmt.Errorf("marshal redis notification: %w", marshalErr))
	}
	if err := fanout.client.Publish(ctx, fanout.channel, envelope).Err(); err != nil {
		return localResult, errors.Join(localErr, fmt.Errorf("publish redis notification: %w", err))
	}
	return localResult, localErr
}

func (fanout *RedisFanout) Close() {
	if fanout == nil {
		return
	}
	fanout.close.Do(func() {
		fanout.cancel()
		fanout.wg.Wait()
	})
}

func (fanout *RedisFanout) subscribeLoop() {
	defer fanout.wg.Done()
	backoff := 100 * time.Millisecond
	for {
		if fanout.ctx.Err() != nil {
			return
		}
		pubsub := fanout.client.Subscribe(fanout.ctx, fanout.channel)
		_, err := pubsub.Receive(fanout.ctx)
		if err == nil {
			for {
				message, receiveErr := pubsub.ReceiveMessage(fanout.ctx)
				if receiveErr != nil {
					err = receiveErr
					break
				}
				fanout.consume(message.Payload)
			}
		}
		_ = pubsub.Close()
		if fanout.ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(backoff)
		select {
		case <-fanout.ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
}

func (fanout *RedisFanout) consume(payload string) {
	var envelope redisNotificationEnvelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil || envelope.Origin == fanout.origin {
		return
	}
	notification := envelope.Notification
	notification.AudienceEmployeePublicID = envelope.AudienceEmployeePublicID
	if notification.Validate() != nil {
		return
	}
	_, _ = fanout.local.Publish(context.Background(), notification)
}
