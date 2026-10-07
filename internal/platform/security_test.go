package platform

import (
	"bytes"
	"testing"
)

func TestPasswordAndEncryption(t *testing.T) {
	if _, e := hashPassword("short"); e == nil {
		t.Fatal("weak password")
	}
	h, e := hashPassword("StrongPass!123")
	if e != nil || !verifyPassword(h, "StrongPass!123") || verifyPassword(h, "wrong") {
		t.Fatal("password verification")
	}
	c := Config{MasterKey: bytes.Repeat([]byte{1}, 32)}
	s, e := c.seal("secret")
	if e != nil || s == "secret" {
		t.Fatal("plaintext encryption")
	}
	if got, e := c.open(s); e != nil || got != "secret" {
		t.Fatal("decrypt")
	}
	s = s[:len(s)-1] + "?"
	if _, e = c.open(s); e == nil {
		t.Fatal("tamper accepted")
	}
}
func TestTicketTransition(t *testing.T) {
	if validTransition("open", "closed") {
		t.Fatal("invalid edge")
	}
	if !validTransition("open", "in_progress") || !validTransition("closed", "open") {
		t.Fatal("valid edge")
	}
}
