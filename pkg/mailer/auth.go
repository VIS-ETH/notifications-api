package mailer

import (
	"fmt"
	"net/smtp"
)

type SMTPAuth struct {
	Username string
	Password string
}

func (a *SMTPAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *SMTPAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.Username), nil
		case "Password:":
			return []byte(a.Password), nil
		default:
			return nil, fmt.Errorf("unkown fromServer: %s", string(fromServer))
		}
	}
	return nil, nil
}
