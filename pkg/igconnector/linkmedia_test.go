package igconnector

import "testing"

func TestLinkMediaParsing(t *testing.T) {
	for _, raw := range []string{"https://instagram.com/reel/B/", "https://www.instagram.com/p/B/?igsh=x", "https://instagram.com/stories/name/123/", "https://instagram.com/stories/highlights/18001704114005607/"} {
		v, e := parseLinkMedia(raw)
		if e != nil || v.ID == "" {
			t.Fatalf("valid link rejected: %v", e)
		}
	}
	for _, raw := range []string{"https://instagram.com.evil.test/p/B/", "http://instagram.com/p/B/", "https://user@instagram.com/p/B/", "https://instagram.com:444/p/B/", "https://instagram.com/p/%FF/", "https://instagram.com/stories/name/nope/"} {
		if _, e := parseLinkMedia(raw); e == nil {
			t.Fatal("invalid link accepted")
		}
	}
}
