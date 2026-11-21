package igconnector

import "testing"

func TestContactAvatarSnapshots(t *testing.T) {
	for _, missing := range []string{"", "%", "/missing.jpg"} {
		if avatar := contactAvatar(missing); avatar != nil {
			t.Fatal("omitted or malformed profile picture should preserve the current avatar")
		}
	}
	if avatar := contactAvatar("https://example.invalid/photos/avatar.jpg?temporary"); avatar == nil || avatar.Remove || avatar.ID != "avatar.jpg" || avatar.Get == nil {
		t.Fatal("a valid picture should replace the current avatar")
	}
	if avatar := wrapAvatar(""); avatar == nil || !avatar.Remove {
		t.Fatal("a thread picture removal should clear the avatar")
	}
}
