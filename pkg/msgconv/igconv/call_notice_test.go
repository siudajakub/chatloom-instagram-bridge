package igconv

import (
	"testing"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
)

func TestVideoCallAdminNotice(t *testing.T) {
	mc := &MessageConverter{}
	const body = "Video call ended"
	part := mc.wrapAdminText(nil, body)
	mc.mutateWrappedAdminText(part, &slidetypes.Message{ContentType: "IG_VIDEO_CALL_XMAT"})
	action := part.Content.BeeperActionMessage
	if part.DontBridge || part.Content.MsgType != event.MsgNotice || part.Content.Body != body ||
		action == nil || action.Type != event.BeeperActionMessageCall || action.CallType != event.BeeperActionMessageCallTypeVideo {
		t.Fatal("expected the original admin notice with video call metadata")
	}

	other := mc.wrapAdminText(nil, body)
	mc.mutateWrappedAdminText(other, &slidetypes.Message{ContentType: "OTHER"})
	if !other.DontBridge || other.Content.BeeperActionMessage != nil {
		t.Fatal("unexpected call metadata on unrelated admin text")
	}
}
