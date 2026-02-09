package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"metawsm/internal/forumbus"
)

type forumCommandDispatcher interface {
	Dispatch(ctx context.Context, topic string, messageKey string, payload any) error
}

type busForumCommandDispatcher struct {
	runtime *forumbus.Runtime
}

func newBusForumCommandDispatcher(runtime *forumbus.Runtime) forumCommandDispatcher {
	return &busForumCommandDispatcher{runtime: runtime}
}

func (d *busForumCommandDispatcher) Dispatch(ctx context.Context, topic string, messageKey string, payload any) error {
	if d == nil || d.runtime == nil {
		return fmt.Errorf("forum dispatcher runtime is not configured")
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("forum dispatcher topic is required")
	}
	if _, err := d.runtime.Publish(topic, strings.TrimSpace(messageKey), payload); err != nil {
		return err
	}
	_, err := d.runtime.ProcessOnce(ctx, 50)
	return err
}
