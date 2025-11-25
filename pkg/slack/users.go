package slack

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type UsernameMap struct {
	expiresAt time.Time
	data      map[string]string
	mu        sync.RWMutex
}

func (u *UsernameMap) Get(username string) (string, bool) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	id, ok := u.data[username]
	return id, ok
}

func (u *UsernameMap) Replace(newData map[string]string, expiresAt time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.expiresAt = expiresAt
	u.data = newData
}

func (c *Client) UpdateUsernameMap(ctx context.Context, api *slack.Client, workspaceURL string) (*UsernameMap, error) {
	tr := otel.Tracer("slack/sender")
	ctx, span := tr.Start(ctx, "slack.getusers",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("slack.workspace-url", workspaceURL),
		),
	)
	defer span.End()

	var users []slack.User
	var err error
	for {
		users, err = api.GetUsersContext(ctx)
		if err == nil {
			break
		}
		err, ok := err.(*slack.RateLimitedError)
		if !ok {
			return nil, fmt.Errorf("failed to get users: %v", err)
		}
		span.AddEvent("rate-limit", trace.WithAttributes(
			attribute.Int("retry-after-seconds", int(err.RetryAfter.Seconds())),
		))
		c.logger.Infof("rate limited while getting users: %v", err)
		<-time.Tick(err.RetryAfter)
	}

	newMap := make(map[string]string)

	for _, user := range users {
		newMap[user.Name] = user.ID
	}

	existingMap, ok := c.workspaceUsers[workspaceURL]
	if !ok || existingMap == nil {
		existingMap = &UsernameMap{data: make(map[string]string)}
		c.workspaceUsers[workspaceURL] = existingMap
	}

	existingMap.Replace(newMap, time.Now().Add(c.updatePeriod))
	span.SetStatus(codes.Ok, "updated users")
	return existingMap, nil
}
