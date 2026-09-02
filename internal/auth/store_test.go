package auth

import "testing"

func TestNormalizePhone(t *testing.T) {
	if got, err := NormalizePhone("+86 13800138000"); err != nil || got != "+8613800138000" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := NormalizePhone("123abc"); err == nil {
		t.Fatal("expected invalid phone")
	}
}
