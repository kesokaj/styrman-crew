package httpapi

import "testing"

func TestEmailIn(t *testing.T) {
	list := []string{"Alice@Gmail.com", "bob@example.org"}
	for _, e := range []string{"alice@gmail.com", "ALICE@GMAIL.COM", "bob@example.org"} {
		if !emailIn(list, e) {
			t.Errorf("emailIn should match %q", e)
		}
	}
	for _, e := range []string{"", "carol@gmail.com", "alice@example.org"} {
		if emailIn(list, e) {
			t.Errorf("emailIn should NOT match %q", e)
		}
	}
}

func TestIsPreviewPath(t *testing.T) {
	yes := []string{
		"/proxy/abc/3000/",
		"/p/abc/1/port/3000/",
		"/p/abc/1/port/8080/some/path",
	}
	no := []string{
		"/", "/api/projects", "/p/abc/1", "/ws/term/abc/1", "/api/projects/abc/ports",
	}
	for _, p := range yes {
		if !isPreviewPath(p) {
			t.Errorf("%q should be a preview path", p)
		}
	}
	for _, p := range no {
		if isPreviewPath(p) {
			t.Errorf("%q should NOT be a preview path", p)
		}
	}
}

func TestEmailValidation(t *testing.T) {
	for _, e := range []string{"a@b.co", "x.y@gmail.com"} {
		if !emailRe.MatchString(e) {
			t.Errorf("%q should be valid", e)
		}
	}
	for _, e := range []string{"nope", "a@b", "@b.co", "a b@c.co"} {
		if emailRe.MatchString(e) {
			t.Errorf("%q should be invalid", e)
		}
	}
}
