package slack

import (
	"time"

	"github.com/sirupsen/logrus"
)

const updatePeriod = 12 * time.Hour

type Client struct {
	// Each workspace (VIS, VSETH, VMP, ...) has their own mapping of username to user
	// We identify workspaces by their URL (example: vmp.slack.com)
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
