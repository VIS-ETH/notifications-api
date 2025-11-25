package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const updatePeriod = 12 * time.Hour

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
		updatePeriod:   updatePeriod,
		logger:         logger,
	}
}

// Send is capable of sending slack blocks https://app.slack.com/block-kit-builder/
func (c *Client) Send(ctx context.Context, token, username, blocksJSON, fallbackText string) error {
	api := slack.New(token)

	tr := otel.Tracer("slack/sender")
	ctx, span := tr.Start(ctx, "slack.send",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	workspaceURL, err := c.getWorkspaceURL(ctx, api)
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
		logrus.Errorf("ok: %t, ussernamemap: %p, expiresAt: %t", ok, usernameMap, usernameMap == nil || time.Now().After(usernameMap.expiresAt))
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
			span.AddEvent("rate-limit", trace.WithAttributes(
				attribute.Int("retry-after-seconds", int(err.RetryAfter.Seconds())),
			))
			<-time.Tick(err.RetryAfter)
		} else {
			break
		}
	}
	span.SetStatus(codes.Ok, "sent slack message")
	return nil
}

func (c *Client) getWorkspaceURL(ctx context.Context, api *slack.Client) (*string, error) {
	tr := otel.Tracer("slack/sender")
	_, span := tr.Start(ctx, "slack.authtest",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

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
		span.AddEvent("rate-limit", trace.WithAttributes(
			attribute.Int("retry-after-seconds", int(err.RetryAfter.Seconds())),
		))
		c.logger.Infof("Got rate limited while getting workspace url: %v", err)
		<-time.Tick(err.RetryAfter)
	}

	span.SetStatus(codes.Ok, "test successful")

	return &authTestResponse.URL, nil
}
