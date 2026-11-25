// SPDX-License-Identifier: AGPL-3.0-or-later

package msgconv

import (
	"fmt"
	"time"

	"go.mau.fi/util/exfmt"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

func CallLogToMatrix(body string, callType event.BeeperActionMessageCallType, status string, duration time.Duration) *bridgev2.ConvertedMessagePart {
	if body == "" {
		switch callType {
		case event.BeeperActionMessageCallTypeVideo:
			body = "Video call"
		case event.BeeperActionMessageCallTypeVoice:
			body = "Voice call"
		default:
			body = "Call"
		}
		switch status {
		case "missed":
			body += " missed"
		case "ended":
			body += " ended"
		}
	}
	fields := map[string]any{"status": status}
	if callType != "" {
		fields["call_type"] = callType
	}
	if duration >= time.Second {
		fields["duration_seconds"] = int64(duration / time.Second)
		body += fmt.Sprintf(" (%s)", exfmt.Duration(duration))
	}
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    body,
			BeeperActionMessage: &event.BeeperActionMessage{
				Type:     event.BeeperActionMessageCall,
				CallType: callType,
			},
		},
		Extra: map[string]any{"com.chatloom.call": fields},
	}
}
