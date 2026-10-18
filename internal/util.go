package internal

import (
	"os"

	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
)

func EnvOrDefault(envVar, defaultVal string) string {
	envVal, exists := os.LookupEnv(envVar)
	if exists {
		return envVal
	}
	return defaultVal
}

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
