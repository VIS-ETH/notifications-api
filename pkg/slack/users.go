package slack

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/slack-go/slack"
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
	users, err := slackCallWrapper(ctx, c, "slack.get-users",
		func(ctx context.Context) ([]slack.User, error) {
			return api.GetUsersContext(ctx)
		})
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %v", err)
	}

	newMap := make(map[string]string)

	for _, user := range *users {
		newMap[user.Name] = user.ID
	}

	existingMap, ok := c.workspaceUsers[workspaceURL]
	if !ok || existingMap == nil {
		existingMap = &UsernameMap{data: make(map[string]string)}
		c.workspaceUsers[workspaceURL] = existingMap
	}

	existingMap.Replace(newMap, time.Now().Add(c.updatePeriod))
	return existingMap, nil
}
