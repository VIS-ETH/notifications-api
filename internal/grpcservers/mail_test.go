package grpcservers_test

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	mockery_sql "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/mockery/generated/sql"
	mockery_mailer "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/mockery/pkg/mailer"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/grpcservers"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

const (
	TestFromName    = "Test From Name"
	TestFromAddress = "test-from@local"
	TestTo1Name     = "Test To1 Name"
	TestTo1Address  = "test-to1@local"

	TestFromAllowedName    = "Test From Name Allowed"
	TestFromAllowedAddress = "test-from-allowed@local"

	TestMailSubject          = "Mail test subject"
	TestMailContent          = "Mail\nTest\rContent\n\ryeah\n\n\n\nTest"
	TestMailContentSanitized = "Mail\nTest\rContent\n\ryeah\n\n\n\nTest"
)

func TestGrpcServerSendMailInvalid(t *testing.T) {
	querier := mockery_sql.NewMockQuerier(t)
	mockMailer := mockery_mailer.NewMockMailSender(t)

	mailGrpcServer := grpcservers.NewMailServer(
		false,
		true,
		querier,
		mockMailer,
	)

	mockMailer.EXPECT().MessageIDSuffix().Return("testing-mail-suffix").Once()
	_, err := mailGrpcServer.SendMail(context.TODO(), &pb.Mail{})
	assert.Error(t, err)
	mockMailer.AssertExpectations(t)

	mockMailer.EXPECT().MessageIDSuffix().Return("testing-mail-suffix").Once()
	_, err = mailGrpcServer.SendMail(context.TODO(), &pb.Mail{
		From: mailer.MakePbAddress(TestFromName, TestFromAddress),
	})
	assert.Error(t, err)
	mockMailer.AssertExpectations(t)

	mockMailer.EXPECT().MessageIDSuffix().Return("testing-mail-suffix").Once()
	_, err = mailGrpcServer.SendMail(context.TODO(), &pb.Mail{
		From: mailer.MakePbAddress(TestFromName, TestFromAddress),
		To:   []*pb.MailAddress{mailer.MakePbAddress(TestTo1Name, TestTo1Address)},
	})
	assert.Error(t, err)
	mockMailer.AssertExpectations(t)

	mockMailer.EXPECT().MessageIDSuffix().Return("testing-mail-suffix").Once()
	_, err = mailGrpcServer.SendMail(context.TODO(), &pb.Mail{
		From:      mailer.MakePbAddress(TestFromName, TestFromAddress),
		Subject:   TestMailSubject,
		BodyOneof: &pb.Mail_PlainText{PlainText: TestMailContent},
	})
	assert.Error(t, err)
	mockMailer.AssertExpectations(t)
}

func TestGrpcServerMailLoggingOnly(t *testing.T) {
	querier := mockery_sql.NewMockQuerier(t)
	mockMailer := mockery_mailer.NewMockMailSender(t)

	mailGrpcServer := grpcservers.NewMailServer(
		true,
		true,
		querier,
		mockMailer,
	)

	from := mailer.MakePbAddress(TestFromName, TestFromAddress)

	to := []*pb.MailAddress{mailer.MakePbAddress(TestTo1Name, TestTo1Address)}

	fromAddr := mail.Address{Name: TestFromName, Address: TestFromAddress}
	calls := []*mock.Call{
		mockMailer.EXPECT().MessageIDSuffix().Return("mail-suffix-logging-only").Call,
		mockMailer.EXPECT().GetSender(fromAddr).Return(fromAddr).Call,
	}
	mock.InOrder(calls...)
	for _, c := range calls {
		c.Once()
	}

	_, err := mailGrpcServer.SendMail(context.TODO(), &pb.Mail{
		From:      from,
		To:        to,
		Subject:   TestMailSubject,
		BodyOneof: &pb.Mail_PlainText{PlainText: TestMailContentSanitized},
	})
	assert.NoError(t, err)
}

func TestGrpcServerSendMailWorks(t *testing.T) {
	querier := mockery_sql.NewMockQuerier(t)
	mockMailer := mockery_mailer.NewMockMailSender(t)

	mailGrpcServer := grpcservers.NewMailServer(
		false,
		true,
		querier,
		mockMailer,
	)

	from := mailer.MakePbAddress(TestFromName, TestFromAddress)

	to := []*pb.MailAddress{mailer.MakePbAddress(TestTo1Name, TestTo1Address)}

	fromAddr := mail.Address{Name: TestFromName, Address: TestFromAddress}

	mailSuffix := "testing-mail-suffix"
	calls := []*mock.Call{
		mockMailer.EXPECT().MessageIDSuffix().Return(mailSuffix).Call,
		mockMailer.EXPECT().GetSender(fromAddr).Return(fromAddr).Call,
		mockMailer.EXPECT().TransmitMail(mock.Anything, mock.Anything).Run(func(_ context.Context, m *mailer.Mail) {
			assert.Equal(t, fromAddr, *m.From)
			assert.Len(t, m.To, 1)
			assert.Equal(t, TestTo1Name, m.To[0].Name)
			assert.Equal(t, TestTo1Address, m.To[0].Address)
			assert.Equal(t, TestMailSubject, m.Subject)
			assert.Equal(t, TestMailContentSanitized, m.Body)
			assert.True(t, strings.HasSuffix(m.MessageID, mailSuffix))
		}).Return(nil).Call,
		querier.EXPECT().CreateMail(mock.Anything, mock.Anything).Return(nil).Call,
	}
	mock.InOrder(calls...)
	for _, c := range calls {
		c.Once()
	}

	_, err := mailGrpcServer.SendMail(context.TODO(), &pb.Mail{
		From:      from,
		To:        to,
		Subject:   TestMailSubject,
		BodyOneof: &pb.Mail_PlainText{PlainText: TestMailContent},
	})
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, mockMailer, querier)

	fromAddr2 := mail.Address{Name: TestFromAllowedName, Address: TestFromAllowedAddress}
	calls = []*mock.Call{
		mockMailer.EXPECT().MessageIDSuffix().Return(mailSuffix).Call,
		mockMailer.EXPECT().GetSender(fromAddr).Return(fromAddr2).Call,
		mockMailer.EXPECT().TransmitMail(mock.Anything, mock.Anything).Run(func(_ context.Context, m *mailer.Mail) {
			assert.Equal(t, fromAddr2, *m.From)
			assert.Len(t, m.To, 1)
			assert.Equal(t, TestTo1Name, m.To[0].Name)
			assert.Equal(t, TestTo1Address, m.To[0].Address)
			assert.Equal(t, TestMailSubject, m.Subject)
			assert.Equal(t, TestMailContentSanitized, m.Body)
			assert.True(t, strings.HasSuffix(m.MessageID, mailSuffix))
		}).Return(nil).Call,
		querier.EXPECT().CreateMail(mock.Anything, mock.Anything).Return(nil).Call,
	}
	mock.InOrder(calls...)
	for _, c := range calls {
		c.Once()
	}

	_, err = mailGrpcServer.SendMail(context.TODO(), &pb.Mail{
		From:      from,
		To:        to,
		Subject:   TestMailSubject,
		BodyOneof: &pb.Mail_PlainText{PlainText: TestMailContentSanitized},
	})
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, mockMailer, querier)
}

func TestTransientErrors(t *testing.T) {
	querier := mockery_sql.NewMockQuerier(t)
	mockMailer := mockery_mailer.NewMockMailSender(t)

	mailGrpcServer := grpcservers.NewMailServer(
		false,
		true,
		querier,
		mockMailer,
	)

	from := mailer.MakePbAddress(TestFromName, TestFromAddress)
	to := []*pb.MailAddress{mailer.MakePbAddress(TestTo1Name, TestTo1Address)}

	mailSuffix := "testing-mail-suffix"
	fromAddr := mail.Address{Name: TestFromAllowedName, Address: TestFromAllowedAddress}
	calls := []*mock.Call{
		mockMailer.EXPECT().MessageIDSuffix().Return(mailSuffix).Call,
		mockMailer.EXPECT().GetSender(mock.Anything).Return(fromAddr).Call,
		mockMailer.EXPECT().TransmitMail(mock.Anything, mock.Anything).Return(errors.New("any")).Call,
	}
	mock.InOrder(calls...)
	for _, c := range calls {
		c.Once()
	}

	_, err := mailGrpcServer.SendMail(context.TODO(), &pb.Mail{
		From:      from,
		To:        to,
		Subject:   TestMailSubject,
		BodyOneof: &pb.Mail_PlainText{PlainText: TestMailContentSanitized},
	})
	assert.Error(t, err)

	mock.AssertExpectationsForObjects(t, mockMailer, querier)

	calls = []*mock.Call{
		mockMailer.EXPECT().MessageIDSuffix().Return(mailSuffix).Call,
		mockMailer.EXPECT().GetSender(mock.Anything).Return(fromAddr).Call,
		mockMailer.EXPECT().TransmitMail(mock.Anything, mock.Anything).Return(nil).Call,
		querier.EXPECT().CreateMail(mock.Anything, mock.Anything).Return(errors.New("error!")).Call,
	}
	mock.InOrder(calls...)
	for _, c := range calls {
		c.Once()
	}

	_, err = mailGrpcServer.SendMail(context.TODO(), &pb.Mail{
		From:      from,
		To:        to,
		Subject:   TestMailSubject,
		BodyOneof: &pb.Mail_PlainText{PlainText: TestMailContentSanitized},
	})
	assert.NoError(t, err)
}
