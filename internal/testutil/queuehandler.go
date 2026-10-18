package testutil

import (
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/dispatcher"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func (te *TestEnvironment) StartQueueHandler(mailer mailer.MailSender, queries *sql.Queries) {
	te.Eg.Go(func() error {
		return dispatcher.HandleMailQueue(te.Ctx, mailer, queries)
	})
}
