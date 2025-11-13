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

func (c *CustomClaims) getRoles() *[]string {
	if c == nil {
		// All grpc methods are auth aware, this is fine (trust me)
		logrus.Warnf("No claim is configured... handling as completely unauthenticated...")
		return nil
	}
	client, ok := c.ResourceAccess[*c.ClientID]
	if !ok {
		logrus.Warnf("No resource_access entry for client %s was found...", *c.ClientID)
		return nil
	}

	roles, ok := client["roles"]
	if !ok {
		logrus.Warnf("No roles were found in resource_access for client %s..", *c.ClientID)
		return nil
	}
	return &roles
}

func (c *CustomClaims) CanMail() bool {
	roles := c.getRoles()
	if roles == nil {
		return false
	}

	return slices.Contains(*roles, "mail")
}

func (c *CustomClaims) IsSenderAllowed(sender *string) bool {
	roles := c.getRoles()
	if roles == nil {
		return false
	}

	return slices.Contains(*roles, "mail-sender:"+*sender)
}

func (c *CustomClaims) IsHeaderAllowed(header *string) bool {
	roles := c.getRoles()
	if roles == nil {
		return false
	}

	return slices.Contains(*roles, "mail-header:"+*header)
}
