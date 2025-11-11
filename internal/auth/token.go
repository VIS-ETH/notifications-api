package auth

import (
	"slices"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

type CustomClaims struct {
	ClientID       *string                        `json:"-"`
	ResourceAccess map[string]map[string][]string `json:"resource_access,omitempty"`
	jwt.RegisteredClaims
}

func (c *CustomClaims) IsSenderAllowed(sender *string) bool {
	client, ok := c.ResourceAccess[*c.ClientID]
	if !ok {
		logrus.Warnf("No resource_access entry for client %s was found...", *c.ClientID)
		return false
	}

	roles, ok := client["roles"]
	if !ok {
		logrus.Warnf("No roles were found in resource_access for client %s..", *c.ClientID)
		return false
	}

	return slices.Contains(roles, *sender)
}
