package main

import "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/cli"

// This includes everything but smaller helpers or debugging/development tools
// Starts notifications API itself, SMTP Proxies, ...

// If you had a tiny container with a single binary to minimize size, these are the
// tools that you'd want included for using it in prod.
func main() {
	cli.Init()
	cli.Execute()
}
