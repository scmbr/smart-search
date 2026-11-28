package searchsession

import "testing"

func TestIdentityRequiresBoundedIDsAndHash(t *testing.T) {
	valid := Identity{Session: "session-1", Key: "request-1", BodyHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if valid.Validate() != nil {
		t.Fatal("valid identity rejected")
	}
	for _, x := range []Identity{{Session: "a", Key: "../bad", BodyHash: valid.BodyHash}, {Session: "", Key: "a", BodyHash: valid.BodyHash}, {Session: "a", Key: "a", BodyHash: "bad"}} {
		if x.Validate() == nil {
			t.Fatal("accepted", x)
		}
	}
}
