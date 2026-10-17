package mailer_test

import (
	"net/mail"
	"strings"
	"testing"
	"time"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func baseMail(body mailer.Part) mailer.Mail {
	loc := time.FixedZone("MADEUP", (4*60+20)*60)
	return mailer.Mail{
		From: mail.Address{
			Name:    "Täst",
			Address: "test@local.ch",
		},
		Date:        time.Date(2004, 4, 1, 2, 3, 4, 5, loc),
		MessageID:   "do-test-1-23@message-id",
		Subject:     "Täst Subjäkt",
		ContentType: "text/plain; charset=utf-8",
		Body:        body,
	}
}

func TestCharsetUtf8With8Bit(t *testing.T) {
	mail := baseMail(&mailer.Text{
		Content: strings.TrimSpace(`
Anöther täst.

Does this work???
`),
	})
	messageContent, err := mail.GetInternetMessageFormat(true)
	if err != nil {
		t.Logf("Error generating message content: %v", err)
		t.Fail()
		return
	}

	expectedContent := strings.ReplaceAll(strings.TrimSpace(`
From: =?utf-8?q?T=C3=A4st?= <test@local.ch>
Date: Thu, 01 Apr 2004 02:03:04 +0420
Message-Id: <do-test-1-23@message-id>
Subject: =?utf-8?q?T=C3=A4st_Subj=C3=A4kt?=
Mime-Version: 1.0
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: 8bit

Anöther täst.

Does this work???
`), "\n", "\r\n")

	if expectedContent != messageContent {
		t.Fatalf("Expected message content:\n%s\n\nGot:\n%s", expectedContent, messageContent)
	}
}

func TestCharsetUtf8QuotedPrintable(t *testing.T) {
	mail := baseMail(&mailer.Text{
		Content: strings.TrimSpace(`
Anöther täst.

Does this work???
`),
	})
	messageContent, err := mail.GetInternetMessageFormat(false)
	if err != nil {
		t.Logf("Error generating message content: %v", err)
		t.Fail()
		return
	}

	expectedContent := strings.ReplaceAll(strings.TrimSpace(`
From: =?utf-8?q?T=C3=A4st?= <test@local.ch>
Date: Thu, 01 Apr 2004 02:03:04 +0420
Message-Id: <do-test-1-23@message-id>
Subject: =?utf-8?q?T=C3=A4st_Subj=C3=A4kt?=
Mime-Version: 1.0
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: quoted-printable

An=C3=B6ther t=C3=A4st.

Does this work???
`), "\n", "\r\n")

	if expectedContent != messageContent {
		t.Fatalf("Expected message content:\n%s\n\nGot:\n%s", expectedContent, messageContent)
	}
}

func TestASCIIBodyUses7Bit(t *testing.T) {
	text := `
	Just plain ascii text.

	should work with whitespace everywhere
	  - including

=> the trailing newline`
	m := baseMail(&mailer.Text{Content: text})

	got, err := m.GetInternetMessageFormat(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "Content-Transfer-Encoding: 7bit") {
		t.Errorf("expected 7bit encoding, got:\n%s", got)
	}
	if !strings.Contains(got, strings.ReplaceAll(text, "\n", "\r\n")) {
		t.Errorf("expected body to be untouched, got:\n%s", got)
	}
}

func TestLongLineForcesSoftNewlines(t *testing.T) {
	// A single line over 998 chars must not be emitted as 7bit even if pure ASCII.
	m := baseMail(&mailer.Text{Content: strings.Repeat("a", 1000)})

	got, err := m.GetInternetMessageFormat(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedContent := strings.ReplaceAll(strings.TrimSpace(`
From: =?utf-8?q?T=C3=A4st?= <test@local.ch>
Date: Thu, 01 Apr 2004 02:03:04 +0420
Message-Id: <do-test-1-23@message-id>
Subject: =?utf-8?q?T=C3=A4st_Subj=C3=A4kt?=
Mime-Version: 1.0
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: quoted-printable

aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
aaaaaaaaaaaaaaaaaaaaaaaaa
`), "\n", "\r\n")

	if expectedContent != got {
		t.Fatalf("Expected message content:\n%s\n\nGot:\n%s", expectedContent, got)
	}
}

func TestEmptyCustomHeaderValueIsSwallowed(t *testing.T) {
	m := baseMail(&mailer.Text{Content: "hi"})
	m.Headers = []mailer.Header{{Key: "X-Empty", Value: ""}, {Key: "X-Set", Value: "val"}}

	got, err := m.GetInternetMessageFormat(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "X-Empty") {
		t.Errorf("did not expect header with empty value to be emitted, got:\n%s", got)
	}
	if !strings.Contains(got, "X-Set: val") {
		t.Errorf("expected header with non-empty value to be emitted, got:\n%s", got)
	}
}

func TestCustomHeaderValueIsEncoded(t *testing.T) {
	m := baseMail(&mailer.Text{Content: "hi"})
	m.Headers = []mailer.Header{{Key: "X-Custom", Value: "Täst"}}

	got, err := m.GetInternetMessageFormat(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "X-Custom: =?utf-8?q?") {
		t.Errorf("expected non-ascii custom header value to be RFC 2047 encoded, got:\n%s", got)
	}
}

func TestMultipartEncodesChildrenWithHeaders(t *testing.T) {
	m := baseMail(&mailer.Multipart{
		Children: []mailer.Child{
			{
				ContentType: "text/plain; charset=utf-8",
				Part:        &mailer.Text{Content: "part one"},
			},
			{
				ContentType: "text/plain; charset=utf-8",
				Part:        &mailer.Text{Content: "part twö"},
			},
			{
				ContentType: "text/plain; charset=utf-8",
				Part:        &mailer.Text{Content: "ääääääääää"},
			},
		},
	})
	m.ContentType = "multipart/mixed; boundary=xyz"

	expectedContent := strings.ReplaceAll(`
From: =?utf-8?q?T=C3=A4st?= <test@local.ch>
Date: Thu, 01 Apr 2004 02:03:04 +0420
Message-Id: <do-test-1-23@message-id>
Subject: =?utf-8?q?T=C3=A4st_Subj=C3=A4kt?=
Mime-Version: 1.0
Content-Type: multipart/mixed; boundary=xyz
Content-Transfer-Encoding: 7bit

--xyz
Content-Transfer-Encoding: 7bit
Content-Type: text/plain; charset=utf-8

part one
--xyz
Content-Transfer-Encoding: quoted-printable
Content-Type: text/plain; charset=utf-8

part tw=C3=B6
--xyz
Content-Transfer-Encoding: base64
Content-Type: text/plain; charset=utf-8

w6TDpMOkw6TDpMOkw6TDpMOkw6Q=
--xyz--
`[1:], "\n", "\r\n")

	got, err := m.GetInternetMessageFormat(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if expectedContent != got {
		t.Fatalf("Expected message content:\n%s\n\nGot:\n%s", expectedContent, got)
	}

	t.Logf("Multipart message content:\n%s", got)
}
