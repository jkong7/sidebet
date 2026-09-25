package mail

import (
	"context"
	"strings"
	"testing"
)

func TestHeaderInjectionRejected(t *testing.T) {
	err := SMTP{Host: "localhost", Port: "1", From: "a@b.c"}.Send(context.Background(), "x@y.z\r\nBcc: evil@x.com", "hi", "body")
	if err == nil || !strings.Contains(err.Error(), "injection") {
		t.Fatalf("err = %v", err)
	}
}

func TestCodeEmail(t *testing.T) {
	subject, body := CodeEmail("Northwestern", "123456")
	if !strings.HasPrefix(subject, "123456") || !strings.Contains(body, "Northwestern") {
		t.Fatalf("%q / %q", subject, body)
	}
}
