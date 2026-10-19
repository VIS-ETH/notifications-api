package mailer

import (
	"bytes"
	"encoding/base64"
	"mime"
	"mime/quotedprintable"
	"strings"
	"unicode/utf8"

	"github.com/sirupsen/logrus"
)

func encodedHeaderVal(v string) string {
	return mime.QEncoding.Encode("utf-8", v)
}

func estimateQuotedPrintableSize(content []byte) int {
	size := 0
	currentLineLen := 0

	for _, c := range content {
		// safe chars are printable ASCII (excluding '='), \t, \r, \n
		isSafe := (c >= 0x20 && c <= 0x7e && c != '=') || c == '\t' || c == '\r' || c == '\n'

		charLen := 1
		if !isSafe {
			charLen = 3
		}

		if c == '\n' {
			size += charLen
			currentLineLen = 0
			continue
		}

		if currentLineLen+charLen > 75 /* 76 bytes limit according to golang docs... */ {
			size += 3 // adds =\r\n, so 3 bytes
			currentLineLen = 0
		}

		size += charLen
		currentLineLen += charLen
	}

	return size
}

func contentTypeRequires7or8Bit(contentType string) bool {
	return strings.HasPrefix(contentType, "message/") || strings.HasPrefix(contentType, "multipart/")
}

func isAcceptable7or8Bit(b []byte) (is7Bit, is8Bit bool) {
	isValidUtf8 := utf8.Valid(b)
	s := string(b)

	if !isValidUtf8 {
		return false, false
	}

	isASCII := true
	hasBadControls := false

	for _, r := range s {
		if r >= 128 {
			isASCII = false
		}
		// printables (mind DEL), allowing \t, \n, \r
		if (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f /* del */ {
			hasBadControls = true
		}
		if !isASCII && hasBadControls {
			break
		}
	}
	needSoftNewLines := false
	for chunk := range strings.SplitSeq(s, "\r\n") {
		if len(chunk) > 998 {
			needSoftNewLines = true
			break
		}
	}

	is7Bit = isASCII && !hasBadControls && !needSoftNewLines
	is8Bit = isValidUtf8 && !hasBadControls && !needSoftNewLines
	return
}

func encodeBytes(has8BitMimeSupport bool, b []byte) (string, string) {
	isAcceptable7Bit, isAcceptable8Bit := isAcceptable7or8Bit(b)

	if isAcceptable7Bit {
		return string(b), "7bit"
	}

	if isAcceptable8Bit {
		return string(b), "8bit"
	}

	estimateQuotedPrintableSize := estimateQuotedPrintableSize(b)

	estimateBase64Raw := 4 * ((len(b) + 2) / 3)
	estimateBase64NewLines := 1 + (estimateBase64Raw / 76)
	estimateBase64 := estimateBase64Raw + estimateBase64NewLines

	logrus.Tracef("estimateQuotedPrintableSize: %d, estimateBase64: %d", estimateQuotedPrintableSize, estimateBase64)

	// choose either base64 or quoted-printable encoding based on size
	if estimateQuotedPrintableSize <= estimateBase64 {
		var buf bytes.Buffer
		w := quotedprintable.NewWriter(&buf)
		_, _ = w.Write(b)
		_ = w.Close()
		return buf.String(), "quoted-printable"
	}

	enc := base64.StdEncoding.EncodeToString(b)

	var sb strings.Builder
	sb.Grow(len(enc) + (len(enc)/76 + 1))
	for len(enc) > 76 {
		sb.WriteString(enc[:76])
		sb.WriteString("\n")
		enc = enc[76:]
	}
	sb.WriteString(enc)
	return sb.String(), "base64"
}
