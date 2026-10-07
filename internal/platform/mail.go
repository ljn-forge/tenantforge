package platform

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

func (a *App) sendOneMail(ctx context.Context) {
	if a.Config.SMTPHost == "" {
		return
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	var id, to, payload string
	var attempts int
	e = tx.QueryRow(ctx, `SELECT id::text,recipient,payload_cipher,attempts FROM mail_outbox WHERE status='pending' AND attempts<8 AND next_attempt_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &to, &payload, &attempts)
	if e != nil {
		return
	}
	claim := digest(randomToken())
	if _, e = tx.Exec(ctx, "UPDATE mail_outbox SET attempts=attempts+1,claim_token=$1,lease_until=now()+interval '60 seconds' WHERE id=$2", claim, id); e != nil {
		return
	}
	if e = tx.Commit(ctx); e != nil {
		return
	}
	plain, e := a.Config.open(payload)
	var mail map[string]string
	if e == nil {
		e = json.Unmarshal([]byte(plain), &mail)
	}
	if e == nil {
		e = a.sendSMTP(ctx, id, to, mail["subject"], mail["body"])
	}
	if e == nil {
		a.DB.Exec(ctx, "UPDATE mail_outbox SET status='sent',payload_cipher='',lease_until=NULL WHERE id=$1 AND claim_token=$2", id, claim)
	} else {
		a.DB.Exec(ctx, "UPDATE mail_outbox SET status=CASE WHEN attempts>=8 THEN 'failed' ELSE 'pending' END,next_attempt_at=now()+($1*interval '30 seconds'),lease_until=NULL WHERE id=$2 AND claim_token=$3", attempts+1, id, claim)
	}
}
func (a *App) sendSMTP(ctx context.Context, id, to, subject, body string) error {
	c := a.Config
	if !emailValid(to) || strings.ContainsAny(subject, "\r\n") {
		return validation("Invalid mail")
	}
	port := envValue(c.SMTPPort, "25")
	conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(c.SMTPHost, port))
	if e != nil {
		return e
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	client, e := smtp.NewClient(conn, c.SMTPHost)
	if e != nil {
		return e
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if e = client.StartTLS(&tls.Config{ServerName: c.SMTPHost, MinVersion: tls.VersionTLS12}); e != nil {
			return e
		}
	} else if c.SMTPUser != "" {
		return fmt.Errorf("SMTP authentication requires TLS")
	}
	if c.SMTPUser != "" {
		if e = client.Auth(smtp.PlainAuth("", c.SMTPUser, c.SMTPPassword, c.SMTPHost)); e != nil {
			return e
		}
	}
	if e = client.Mail(c.SMTPFrom); e != nil {
		return e
	}
	if e = client.Rcpt(to); e != nil {
		return e
	}
	w, e := client.Data()
	if e != nil {
		return e
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s@tenantforge>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", c.SMTPFrom, to, subject, id, strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	if _, e = w.Write([]byte(message)); e != nil {
		return e
	}
	if e = w.Close(); e != nil {
		return e
	}
	return client.Quit()
}
