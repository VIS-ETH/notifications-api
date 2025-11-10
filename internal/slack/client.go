package slack

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/slack-go/slack"
)

type UsernameMap struct {
	data map[string]string
	mu   sync.RWMutex
}

func (u *UsernameMap) Get(username string) (string, bool) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	id, ok := u.data[username]
	return id, ok
}

func (u *UsernameMap) Replace(newData map[string]string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.data = newData
}

type Client struct {
	api            *slack.Client
	backoffInitial time.Duration
	backoffMax     time.Duration
	// Maps ethz usernams to slack ids
	usernameMap  *UsernameMap
	updatePeriod time.Duration
}

func (c *Client) UpdateUsernameMap() {
	users, err := c.api.GetUsers()
	if err != nil {
		panic("GetUsers failed restart the pod")
	}

	newMap := make(map[string]string)

	for _, user := range users {
		newMap[user.Name] = user.ID
	}

	c.usernameMap.Replace(newMap)

}

func (c *Client) ScheduleUpdateUsernameMap() {
	go func() {
		for {
			c.UpdateUsernameMap()
			time.Sleep(c.updatePeriod)
		}
	}()
}

func NewClient(slackSecret string) *Client {
	u := &UsernameMap{data: make(map[string]string)}
	c := &Client{api: slack.New(slackSecret), backoffInitial: 100 * time.Millisecond, backoffMax: 2000 * time.Millisecond, usernameMap: u}
	c.ScheduleUpdateUsernameMap()
	return c
}

func (c *Client) Send(ctx context.Context, username, blocksJson, fallbackText string) error {
	// Method capable of sending slack blocks https://app.slack.com/block-kit-builder/

	// Ok i do not know why this wrapper was necessary but otherwise unmarshalling would simply not work
	var wrapper struct {
		Blocks json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(blocksJson), &wrapper); err != nil {
		return err
	}
	var blocks slack.Blocks
	if err := blocks.UnmarshalJSON(wrapper.Blocks); err != nil {
		return err
	}

	userId, ok := c.usernameMap.Get(username)
	if !ok {
		return errors.New("user not found")
	}

	// Then find user via email and deal with sending message
	op := func() error {
		_, _, err := c.api.PostMessageContext(ctx, userId, slack.MsgOptionBlocks(blocks.BlockSet...), slack.MsgOptionText(fallbackText, false))
		return err
	}

	// Done with exponential backoff
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = c.backoffInitial
	b.MaxElapsedTime = c.backoffMax

	if err := backoff.RetryNotify(op, b, func(err error, d time.Duration) {
		log.Printf("retry in %s due to %v", d, err)
	}); err != nil {
		return errors.New("failed after retries: " + err.Error())
	}
	return nil
}
