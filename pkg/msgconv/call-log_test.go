// SPDX-License-Identifier: AGPL-3.0-or-later

package msgconv

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
)

func TestMessengerCallLog(t *testing.T) {
	part := CallLogToMatrix("", event.BeeperActionMessageCallTypeVoice, "ended", 62*time.Second)
	fields := part.Extra["com.chatloom.call"].(map[string]any)
	if fields["status"] != "ended" || fields["duration_seconds"] != int64(62) ||
		part.Content.BeeperActionMessage.CallType != event.BeeperActionMessageCallTypeVoice {
		t.Fatal("call log lost the media type, outcome or duration")
	}
}
