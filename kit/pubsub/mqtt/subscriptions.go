package mqtt

import (
	"context"
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/eclipse/paho.golang/paho"
)

// resubscribeTimeout bounds each attempt to restore the subscriptions after reconnecting.
const resubscribeTimeout = 5 * time.Second

// subscribe remembers successful subscriptions for the next connection, independently of ctx.
func (c *Client) subscribe(ctx context.Context, subscription paho.SubscribeOptions) error {
	c.subscriptionsLock.Lock()
	generation := c.generation
	c.subscriptionsLock.Unlock()

	_, err := c.Subscribe(ctx, &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{subscription}})
	if err != nil {
		return err
	}

	c.subscriptionsLock.Lock()
	c.subscriptions[subscription.Topic] = subscription
	reconnected := generation != c.generation
	c.subscriptionsLock.Unlock()
	if reconnected {
		// A reconnect may have taken its snapshot before this successful subscription was remembered.
		c.requestRestore()
	}
	return nil
}

// onReconnect must not block autopaho's connection loop on network operations.
func (c *Client) onReconnect() {
	c.subscriptionsLock.Lock()
	c.generation++
	c.subscriptionsLock.Unlock()
	c.requestRestore()
}

func (c *Client) onConnectionDown() bool {
	c.subscriptionsLock.Lock()
	defer c.subscriptionsLock.Unlock()
	// A lost SUBACK can leave Subscribe waiting until its timeout. Release the worker
	// immediately so it can restore subscriptions as soon as the next connection is up.
	if c.restoreCancel != nil {
		c.restoreCancel()
	}
	return true
}

func (c *Client) requestRestore() {
	// Coalesce reconnects while the single worker is already restoring subscriptions.
	select {
	case c.restoreRequested <- struct{}{}:
	default:
	}
}

func (c *Client) restoreSubscriptions(ctx context.Context, l *log.Logger, retryDelay time.Duration) {
	defer close(c.restoreDone)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.restoreRequested:
		}
		for {
			if err := c.AwaitConnection(ctx); err != nil || ctx.Err() != nil {
				return
			}
			err := c.resubscribe(ctx)
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				return
			}
			l.Warn(ctx, "restoring MQTT subscriptions", log.Error(err))

			timer := time.NewTimer(retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-c.restoreRequested:
				timer.Stop()
			case <-timer.C:
			}
		}
	}
}

func (c *Client) resubscribe(ctx context.Context) error {
	c.subscriptionsLock.Lock()
	subscriptions := make([]paho.SubscribeOptions, 0, len(c.subscriptions))
	for _, subscription := range c.subscriptions {
		subscriptions = append(subscriptions, subscription)
	}
	if len(subscriptions) == 0 {
		c.subscriptionsLock.Unlock()
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, resubscribeTimeout)
	c.restoreCancel = cancel
	c.subscriptionsLock.Unlock()
	defer func() {
		cancel()
		c.subscriptionsLock.Lock()
		c.restoreCancel = nil
		c.subscriptionsLock.Unlock()
	}()
	// Reuse the existing routes: registering handlers again would duplicate message delivery.
	_, err := c.Subscribe(ctx, &paho.Subscribe{Subscriptions: subscriptions})
	return err
}
