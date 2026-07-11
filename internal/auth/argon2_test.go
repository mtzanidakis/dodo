package auth

import "testing"

func TestVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("correct password should verify")
	}
	if VerifyPassword("wrong", hash) {
		t.Fatal("wrong password must not verify")
	}
	if VerifyPassword("", hash) {
		t.Fatal("empty password must not verify")
	}
}

// An empty encoded hash means "no such user"; verification must always fail,
// even when the supplied password equals the internal timing-equalizer value.
func TestVerifyPasswordEmptyHashNeverAuthenticates(t *testing.T) {
	if VerifyPassword("timing-equalizer-never-a-real-password", "") {
		t.Fatal("empty hash must never authenticate")
	}
	if VerifyPassword("anything", "") {
		t.Fatal("empty hash must never authenticate")
	}
	if VerifyPassword("anything", "not-a-valid-encoding") {
		t.Fatal("malformed hash must never authenticate")
	}
}
