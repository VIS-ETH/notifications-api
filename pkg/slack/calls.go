package slack

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type SlackContextKeys string

const workspaceURLContextKey SlackContextKeys = "slack-call-wrapper-url-context-key"

const maxRetries = 20

func withWorkspaceURL(ctx context.Context, workspaceURL string) context.Context {
	return context.WithValue(ctx, workspaceURLContextKey, workspaceURL)
}

// slackCallWrapper automatically retries with ratelimit (mind the delay!).
// It also takes care of observability and tracing, as well as logging etc.
func slackCallWrapper[R any](
	ctx context.Context,
	client *Client,
	spanName string,
	f func(context.Context) (R, error),
) (*R, error) {
	logger := client.logger.WithFields(logrus.Fields{
		"slack-call-wrapper-span": spanName,
	})
	workspaceURL := ctx.Value(workspaceURLContextKey)
	if workspaceURL == nil {
		workspaceURL = "unknown"
	}
	workspaceURLStr, ok := workspaceURL.(string)
	if !ok {
		workspaceURLStr = "unknown"
	}

	tr := otel.Tracer("slack/call-wrapper")
	ctx, span := tr.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("slack.workspace-url", workspaceURLStr),
		),
	)
	defer span.End()

	var res R
	var err error

	for range maxRetries {
		res, err = f(ctx)
		if err != nil {
			rateLimitErr, ok := err.(*slack.RateLimitedError)
			if !ok {
				logger.Errorf("request to Slack API failed: %v", err)
				return nil, fmt.Errorf("failed to make slack request: %v", err)
			}
			retryAfterSeconds := max(int(rateLimitErr.RetryAfter.Seconds()), 1) // wait at least 1 second...
			span.AddEvent("rate-limit", trace.WithAttributes(
				attribute.Int("retry-after-seconds", retryAfterSeconds),
			))
			logger.Infof("Rate limited at Slack API call, retrying after %d seconds: ", retryAfterSeconds)
			<-time.Tick(max(rateLimitErr.RetryAfter, 1*time.Second))
		} else {
			break
		}
	}
	span.SetStatus(codes.Ok, "success")

	return &res, nil
}
