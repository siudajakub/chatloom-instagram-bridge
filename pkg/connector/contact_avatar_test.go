package connector

import "testing"

func TestContactAvatarSnapshots(t *testing.T) {
	if avatar := contactAvatar(""); avatar != nil {
		t.Fatal("omitted profile picture should preserve the current avatar")
	}
	if avatar := contactAvatar("%"); avatar != nil {
		t.Fatal("malformed profile picture should preserve the current avatar")
	}
	if avatar := contactAvatar("https://example.invalid/photos/avatar.jpg"); avatar == nil || avatar.Remove || avatar.ID != "avatar.jpg" {
		t.Fatal("a valid picture should replace the current avatar")
	}
	if avatar := wrapAvatar(""); avatar == nil || !avatar.Remove {
		t.Fatal("a thread picture removal should clear the avatar")
	}
}
