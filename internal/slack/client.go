package slack

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/slack-go/slack"
)

type Client struct {
	api              *slack.Client
	backoffInitialMs int
	backoffMaxMs     int
}

func NewClient(slackSecret string) *Client {
	return &Client{api: slack.New(slackSecret), backoffInitialMs: 100, backoffMaxMs: 2000}
}

func (c *Client) Send(ctx context.Context, email, msg string) error {
	op := func() error {
		// Looking up user id via email
		u, err := c.api.GetUserByEmail(email)
		if err != nil {
			return err
		}
		_, _, err = c.api.PostMessageContext(ctx, u.ID, slack.MsgOptionText(msg, false))
		return err
	}

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
