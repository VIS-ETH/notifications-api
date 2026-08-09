package smtpproxy

import (
	"context"
	"fmt"

	"github.com/emersion/go-smtp"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/smtpserver"
)

func GetSMTPServer(client pb.MailServiceClient) *smtp.Server {
	return smtp.NewServer(
		smtpserver.NewBackend(func(ctx context.Context, mail *mailer.Mail) error {
			to := make([]*pb.MailAddress, len(mail.To))
			for i, addr := range mail.To {
				to[i] = mailer.MakePbAddress(addr.Name, addr.Address)
			}
			cc := make([]*pb.MailAddress, len(mail.Cc))
			for i, addr := range mail.Cc {
				cc[i] = mailer.MakePbAddress(addr.Name, addr.Address)
			}
			bcc := make([]*pb.MailAddress, len(mail.Bcc))
			for i, addr := range mail.Bcc {
				bcc[i] = mailer.MakePbAddress(addr.Name, addr.Address)
			}
			replyTo := make([]*pb.MailAddress, len(mail.ReplyTo))
			for i, addr := range mail.ReplyTo {
				replyTo[i] = mailer.MakePbAddress(addr.Name, addr.Address)
			}

			var extraHeaders []*pb.ExtraHeader
			for k, vs := range mail.ExtraHeaders {
				for _, v := range vs {
					extraHeaders = append(extraHeaders, &pb.ExtraHeader{Key: k, Value: v})
				}
			}

			pbMail := &pb.Mail{
				Subject:     mail.Subject,
				From:        mailer.MakePbAddress(mail.From.Name, mail.From.Address),
				To:          to,
				Cc:          cc,
				Bcc:         bcc,
				ReplyTo:     replyTo,
				ExtraHeader: extraHeaders,
				BodyOneof: &pb.Mail_PlainText{
					PlainText: mail.Body,
				},
			}
			/*resp*/ _, err := client.SendMail(ctx, pbMail)
			if err != nil {
				return fmt.Errorf("failed to send mail via gRPC: %v", err)
			}
			return nil
		}))
}
