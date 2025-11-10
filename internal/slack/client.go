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

type Client struct {
	api              *slack.Client
	backoffInitialMs int
	backoffMaxMs     int
	// Maps ethz usernams to slack ids
	usernameMap sync.Map
}

func (c *Client) UpdateUsernameMap() {
	users, err := c.api.GetUsers()
	if err != nil {
		panic("GetUsers failed restart the pod")
	}

	c.usernameMap.Clear()

	for _, user := range users {
		c.usernameMap.Store(user.Name, user.ID)
	}
}

func ScheduleUpdateUsernameMap(c *Client) {
	go func() {
		for {
			c.UpdateUsernameMap()
			time.Sleep(2 * time.Hour)
		}
	}()
}

func NewClient(slackSecret string) *Client {
	c := &Client{api: slack.New(slackSecret), backoffInitialMs: 100, backoffMaxMs: 2000, usernameMap: sync.Map{}}
	ScheduleUpdateUsernameMap(c)
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

	// Then find user via email and deal with sending message
	op := func() error {
		userId, ok := c.usernameMap.Load(username)
		if !ok {
			return errors.New("user not found")
		}

		_, _, err := c.api.PostMessageContext(ctx, userId.(string), slack.MsgOptionBlocks(blocks.BlockSet...), slack.MsgOptionText(fallbackText, false))
		return err
	}

	// Done with exponential backoff
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = time.Duration(c.backoffInitialMs) * time.Millisecond
	b.MaxElapsedTime = time.Duration(c.backoffMaxMs) * time.Millisecond

	if err := backoff.RetryNotify(op, b, func(err error, d time.Duration) {
		log.Printf("retry in %s due to %v", d, err)
	}); err != nil {
		return errors.New("failed after retries: " + err.Error())
	}
	return nil
}
