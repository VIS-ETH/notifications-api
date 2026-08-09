package mailer

import pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"

func MakePbAddress(name, address string) *pb.MailAddress {
	return &pb.MailAddress{
		Address: &pb.MailAddress_MailAddress{
			MailAddress: &pb.MailAddress_Address{
				Address: address,
				Name:    name,
			},
		},
	}
}
