package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// SendToUsername is capable of sending slack blocks https://app.slack.com/block-kit-builder/
func (c *Client) SendToUsername(ctx context.Context, token, username, blocksJSON, fallbackText string) error {
	api := slack.New(token, slack.OptionHTTPClient(&http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}))

	// could be cached, but might be better to not store tokens in a hashmap...
	authTestRes, err := slackCallWrapper(ctx, c, "slack.auth-test",
		func(ctx context.Context) (*slack.AuthTestResponse, error) {
			return api.AuthTestContext(ctx)
		})
	if err != nil {
		return fmt.Errorf("failed to get workspace url: %v", err)
	}
	if authTestRes == nil || *authTestRes == nil {
		return errors.New("auth test response was nil")
	}
	workspaceURL := (*authTestRes).URL
	ctx = withWorkspaceURL(ctx, workspaceURL)
	logger := c.logger.WithFields(logrus.Fields{
		"receiver-username": username,
		"workspace":         workspaceURL,
	})

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

	usernameMap, ok := c.workspaceUsers[workspaceURL]
	if !ok || usernameMap == nil || time.Now().After(usernameMap.expiresAt) {
		usernameMap, err = c.UpdateUsernameMap(ctx, api, workspaceURL)
		if err != nil {
			return fmt.Errorf("failed to fetch list of users: %v", err)
		}
	}

	userID, ok := usernameMap.Get(username)
	if !ok {
		return errors.New("user not found")
	}

	_, err = slackCallWrapper(ctx, c, "slack.post-message",
		func(ctx context.Context) (*any, error) {
			_, _, err := api.PostMessageContext(ctx, userID,
				slack.MsgOptionBlocks(blocks.BlockSet...),
				slack.MsgOptionText(fallbackText, false),
			)
			return nil, err
		})
	if err != nil {
		return fmt.Errorf("failed to post slack message: %v", err)
	}

	logger.Infof("Slack Message sent successfully")
	return nil
}
