package email_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/email"
)

// fakeSMTP starts a minimal plain SMTP server on a random local port.
// It handles exactly one connection: 220 → EHLO → MAIL FROM → RCPT TO → DATA → QUIT.
// Call messages() after Send() to get the collected message bodies.
func fakeSMTP(t *testing.T) (port int, messages func() []string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("fakeSMTP: listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	var mu sync.Mutex
	var collected []string

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		defer conn.Close()
		handleSMTP(conn, &mu, &collected)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return addr.Port, func() []string {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]string, len(collected))
		copy(cp, collected)
		return cp
	}
}

// handleSMTP speaks minimal SMTP (no TLS, no AUTH) and collects message bodies.
func handleSMTP(conn net.Conn, mu *sync.Mutex, collected *[]string) {
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	send := func(line string) {
		fmt.Fprintf(rw, "%s\r\n", line)
		rw.Flush()
	}

	readLine := func() string {
		line, _ := rw.ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}

	// Greeting
	send("220 localhost ESMTP fakeSMTP")

	// Expect EHLO
	line := readLine()
	if strings.HasPrefix(line, "EHLO") || strings.HasPrefix(line, "HELO") {
		send("250-localhost")
		send("250 OK")
	}

	// Expect MAIL FROM
	line = readLine()
	if strings.HasPrefix(strings.ToUpper(line), "MAIL FROM") {
		send("250 OK")
	}

	// Expect RCPT TO
	line = readLine()
	if strings.HasPrefix(strings.ToUpper(line), "RCPT TO") {
		send("250 OK")
	}

	// Expect DATA
	line = readLine()
	if strings.ToUpper(strings.TrimSpace(line)) == "DATA" {
		send("354 Start mail input; end with <CRLF>.<CRLF>")

		// Collect body until ".\r\n"
		var body strings.Builder
		for {
			l, err := rw.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					// read error
				}
				break
			}
			stripped := strings.TrimRight(l, "\r\n")
			if stripped == "." {
				break
			}
			body.WriteString(l)
		}

		mu.Lock()
		*collected = append(*collected, body.String())
		mu.Unlock()

		send("250 OK")
	}

	// Expect QUIT
	line = readLine()
	if strings.ToUpper(strings.TrimSpace(line)) == "QUIT" {
		send("221 Bye")
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestSend_NoAuth(t *testing.T) {
	port, msgs := fakeSMTP(t)

	cfg := email.Config{
		Host: "127.0.0.1",
		Port: port,
		From: "ding@localhost",
		To:   "alert@example.com",
	}
	changes := []diff.Change{
		{Kind: diff.KindNew, IP: "192.168.1.50", Desc: "mac=aa:bb:cc:dd:ee:ff"},
	}

	if err := email.Send(cfg, changes); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := msgs()
	if len(got) != 1 {
		t.Fatalf("want 1 message delivered, got %d", len(got))
	}
	msg := got[0]
	if !strings.Contains(msg, "From:") {
		t.Error("message missing From header")
	}
	if !strings.Contains(msg, "To: alert@example.com") {
		t.Error("message missing To header")
	}
	if !strings.Contains(msg, "Subject:") {
		t.Error("message missing Subject header")
	}
}

func TestSend_NoHost(t *testing.T) {
	cfg := email.Config{
		Host: "",
		Port: 587,
		To:   "alert@example.com",
	}
	err := email.Send(cfg, nil)
	if err == nil {
		t.Error("expected error for empty host, got nil")
	}
}

func TestSendTest_NoHost(t *testing.T) {
	cfg := email.Config{
		Host: "",
		Port: 587,
		To:   "alert@example.com",
	}
	err := email.SendTest(cfg)
	if err == nil {
		t.Error("expected error for empty host, got nil")
	}
}

func TestBuildContent_SingleChange(t *testing.T) {
	port, msgs := fakeSMTP(t)

	cfg := email.Config{
		Host: "127.0.0.1",
		Port: port,
		From: "ding@localhost",
		To:   "alert@example.com",
	}
	changes := []diff.Change{
		{Kind: diff.KindNew, IP: "10.0.0.1", Desc: "mac=aa:bb:cc:00:00:01"},
	}
	if err := email.Send(cfg, changes); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := msgs()
	if len(got) != 1 {
		t.Fatalf("want 1 message, got %d", len(got))
	}
	if !strings.Contains(got[0], "1 network change") {
		t.Errorf("subject: want '1 network change', message:\n%s", got[0])
	}
}

func TestBuildContent_MultipleChanges(t *testing.T) {
	port, msgs := fakeSMTP(t)

	cfg := email.Config{
		Host: "127.0.0.1",
		Port: port,
		From: "ding@localhost",
		To:   "alert@example.com",
	}
	changes := []diff.Change{
		{Kind: diff.KindNew, IP: "10.0.0.1", Desc: "mac=aa:bb:cc:00:00:01"},
		{Kind: diff.KindGone, IP: "10.0.0.2", Desc: "no longer visible"},
		{Kind: diff.KindBack, IP: "10.0.0.3", Desc: "mac=aa:bb:cc:00:00:03"},
	}
	if err := email.Send(cfg, changes); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := msgs()
	if len(got) != 1 {
		t.Fatalf("want 1 message, got %d", len(got))
	}
	if !strings.Contains(got[0], "3 network changes") {
		t.Errorf("subject: want '3 network changes', message:\n%s", got[0])
	}
}

func TestBuildContent_Body(t *testing.T) {
	port, msgs := fakeSMTP(t)

	cfg := email.Config{
		Host: "127.0.0.1",
		Port: port,
		From: "ding@localhost",
		To:   "alert@example.com",
	}
	changes := []diff.Change{
		{Kind: diff.KindNew, IP: "192.168.1.99", Desc: "mac=de:ad:be:ef:00:01"},
	}
	if err := email.Send(cfg, changes); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := msgs()
	if len(got) != 1 {
		t.Fatalf("want 1 message, got %d", len(got))
	}
	if !strings.Contains(got[0], "192.168.1.99") {
		t.Errorf("body: want IP 192.168.1.99, message:\n%s", got[0])
	}
	if !strings.Contains(got[0], "NEW") {
		t.Errorf("body: want kind NEW, message:\n%s", got[0])
	}
}
