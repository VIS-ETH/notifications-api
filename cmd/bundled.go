package main

import (
	"os"

	"github.com/sirupsen/logrus"
)

func main() {
	logrus.Info("Running Notifications API CLI!")

	if len(os.Args) < 2 {
		logrus.Fatalf("No command provided. Available commands: notifications-api, smtp-proxy, mail-sender")
	}

	switch os.Args[1] {
	case "notifications-api":
		break
	case "smtp-proxy":
		break
	case "mail-sender":
		break
	default:
		logrus.Fatalf("Unknown command: %s. Available commands: notifications-api, smtp-proxy, mail-sender", os.Args[1])
	}
}
