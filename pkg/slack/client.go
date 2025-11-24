package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

type Client struct {
	// Each workspace (VIS, VSETH, VMP, ...) has their own mapping of username to user
	workspaceUsers map[string]*UsernameMap
	updatePeriod   time.Duration

	logger *logrus.Entry
}

func NewClient() *Client {
	logger := logrus.WithFields(logrus.Fields{
		"component": "slack-sender",
	})
	return &Client{
		workspaceUsers: make(map[string]*UsernameMap),
		logger:         logger,
	}
}

// Send is capable of sending slack blocks https://app.slack.com/block-kit-builder/
func (c *Client) Send(ctx context.Context, token, username, blocksJSON, fallbackText string) error {
	api := slack.New(token)

	workspaceURL, err := c.getWorkspaceURL(api)
	if err != nil {
		return fmt.Errorf("failed to get workspace url: %v", err)
	}

	// Ok i do not know why this wrapper was necessary but otherwise unmarshalling would simply not work
	var wrapper struct {
		Blocks json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(blocksJSON), &wrapper); err != nil {
		return err
	}
	var blocks slack.Blocks
	if err := blocks.UnmarshalJSON(wrapper.Blocks); err != nil {
		return err
	}

	usernameMap, ok := c.workspaceUsers[*workspaceURL]
	if !ok || usernameMap == nil || time.Now().After(usernameMap.expiresAt) {
		usernameMap, err = c.UpdateUsernameMap(ctx, api, *workspaceURL)
		if err != nil {
			return fmt.Errorf("failed to fetch list of users: %v", err)
		}
	}

	userID, ok := usernameMap.Get(username)
	if !ok {
		return errors.New("user not found")
	}

	for {
		_, _, err := api.PostMessageContext(ctx, userID, slack.MsgOptionBlocks(blocks.BlockSet...), slack.MsgOptionText(fallbackText, false))
		if err != nil {
			err, ok := err.(*slack.RateLimitedError)
			if !ok {
				return fmt.Errorf("failed to post message: %v", err)
			}
			<-time.Tick(err.RetryAfter)
		} else {
			break
		}
	}
	return nil
}

func (c *Client) getWorkspaceURL(api *slack.Client) (*string, error) {
	var authTestResponse *slack.AuthTestResponse
	var err error
	for {
		authTestResponse, err = api.AuthTest()
		if err == nil {
			break
		}
		err, ok := err.(*slack.RateLimitedError)
		if !ok {
			return nil, fmt.Errorf("error while testing authentication: %v", err)
		}
		c.logger.Infof("Got rate limited while getting workspace url: %v", err)
		<-time.Tick(err.RetryAfter)
	}

	return &authTestResponse.URL, nil
}
