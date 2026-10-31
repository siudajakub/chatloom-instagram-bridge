// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package igconv

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
)

// fakeMatrix answers the one MatrixConnector call direct media makes; the
// other methods are never reached by these tests.
type fakeMatrix struct {
	bridgev2.MatrixConnector
}

func (fakeMatrix) GenerateContentURI(context.Context, networkid.MediaID) (id.ContentURIString, error) {
	return "mxc://example.invalid/raven", nil
}

// convertRaven runs a fictional Instagram message through ToMatrix with
// direct media, so nothing is downloaded or uploaded.
func convertRaven(t *testing.T, mc *MessageConverter, contentJSON string) *bridgev2.ConvertedMessagePart {
	t.Helper()
	var msg slidetypes.Message
	if err := json.Unmarshal([]byte(`{"content":`+contentJSON+`}`), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	portal := &bridgev2.Portal{
		Portal: &database.Portal{},
		Bridge: &bridgev2.Bridge{Matrix: fakeMatrix{}},
	}
	login := &bridgev2.UserLogin{UserLogin: &database.UserLogin{ID: "1001"}}
	cm := mc.ToMatrix(context.Background(), portal, nil, login, nil, "mid.raven", &msg, true)
	if len(cm.Parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(cm.Parts))
	}
	return cm.Parts[0]
}

const ravenAttachment = `{
	"__typename": "SlideMessagingMediaAttachment",
	"attachment_fbid": "900",
	"attachment_cdn_url": "https://cdn.example.invalid/v/raven.jpg?oe=6A000000",
	"preview_width": 720,
	"preview_height": 1280
}`

func ravenContent(typename string, viewMode int, withAttachment bool) string {
	att := "null"
	if withAttachment {
		att = ravenAttachment
	}
	b, _ := json.Marshal(map[string]any{
		"__typename": typename,
		"view_mode":  viewMode,
		"attachment": json.RawMessage(att),
	})
	return string(b)
}

func ravenMarker(t *testing.T, part *bridgev2.ConvertedMessagePart) (map[string]any, bool) {
	t.Helper()
	raw, ok := part.Extra[ChatLoomRavenKey]
	if !ok {
		return nil, false
	}
	marker, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("marker has type %T, want a map", raw)
	}
	return marker, true
}

func assertNoBeeperViewLimited(t *testing.T, part *bridgev2.ConvertedMessagePart) {
	t.Helper()
	if _, ok := part.Extra["com.beeper.view_limited"]; ok {
		t.Fatal("com.beeper.view_limited must not be set: it opts into bridgev2 view-limited flows")
	}
	if part.Content.BeeperViewLimited != nil {
		t.Fatal("content.BeeperViewLimited must stay nil")
	}
}

func TestRavenViewOnceImageIsBridgedWithMarker(t *testing.T) {
	part := convertRaven(t, &MessageConverter{DirectMedia: true}, ravenContent("SlideMessageRavenImageContent", 0, true))
	if part.Content.MsgType != event.MsgImage {
		t.Fatalf("msgtype %q, want m.image", part.Content.MsgType)
	}
	if part.Content.URL == "" {
		t.Fatal("bridged image has no URL")
	}
	marker, ok := ravenMarker(t, part)
	if !ok {
		t.Fatal("view-once image has no ChatLoom raven marker")
	}
	if marker["version"] != ChatLoomRavenVersion || marker["view_mode"] != 0 {
		t.Fatalf("marker %v, want version 1 view_mode 0", marker)
	}
	assertNoBeeperViewLimited(t, part)
}

func TestRavenAllowReplayVideoIsBridgedWithMarker(t *testing.T) {
	part := convertRaven(t, &MessageConverter{DirectMedia: true}, ravenContent("SlideMessageRavenVideoContent", 1, true))
	if part.Content.MsgType != event.MsgVideo {
		t.Fatalf("msgtype %q, want m.video", part.Content.MsgType)
	}
	marker, ok := ravenMarker(t, part)
	if !ok {
		t.Fatal("allow-replay video has no ChatLoom raven marker")
	}
	if marker["version"] != ChatLoomRavenVersion || marker["view_mode"] != 1 {
		t.Fatalf("marker %v, want version 1 view_mode 1", marker)
	}
	assertNoBeeperViewLimited(t, part)
}

func TestRavenMarkerSerialisesAtTopLevel(t *testing.T) {
	part := convertRaven(t, &MessageConverter{DirectMedia: true}, ravenContent("SlideMessageRavenImageContent", 1, true))
	wrapped := &event.Content{Parsed: part.Content, Raw: part.Extra}
	b, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	marker, ok := decoded[ChatLoomRavenKey].(map[string]any)
	if !ok {
		t.Fatalf("event content has no %s object", ChatLoomRavenKey)
	}
	if marker["version"] != float64(1) || marker["view_mode"] != float64(1) {
		t.Fatalf("serialised marker %v", marker)
	}
	if _, ok := decoded["com.beeper.view_limited"]; ok {
		t.Fatal("serialised content has com.beeper.view_limited")
	}
}

func TestRavenKeepInChatHasNoMarker(t *testing.T) {
	for _, typename := range []string{"SlideMessageRavenImageContent", "SlideMessageRavenVideoContent"} {
		part := convertRaven(t, &MessageConverter{DirectMedia: true}, ravenContent(typename, 2, true))
		if part.Content.MsgType != event.MsgImage && part.Content.MsgType != event.MsgVideo {
			t.Fatalf("%s: msgtype %q, want media", typename, part.Content.MsgType)
		}
		if _, ok := ravenMarker(t, part); ok {
			t.Fatalf("%s: keep-in-chat media must not carry the raven marker", typename)
		}
		assertNoBeeperViewLimited(t, part)
	}
}

func TestRavenWithoutAttachmentKeepsNotice(t *testing.T) {
	cases := []struct {
		typename string
		mode     int
		want     string
	}{
		{"SlideMessageRavenImageContent", 0, "This image can only be viewed once. Use the Instagram app to view."},
		{"SlideMessageRavenVideoContent", 1, "This video can only be replayed once. Use the Instagram app to view."},
	}
	for _, c := range cases {
		part := convertRaven(t, &MessageConverter{DirectMedia: true}, ravenContent(c.typename, c.mode, false))
		if part.Content.MsgType != event.MsgNotice || part.Content.Body != c.want {
			t.Fatalf("%s/%d: got %q %q", c.typename, c.mode, part.Content.MsgType, part.Content.Body)
		}
		if _, ok := ravenMarker(t, part); ok {
			t.Fatalf("%s/%d: notice must not carry the raven marker", c.typename, c.mode)
		}
	}
}

func TestRavenDisableViewOnceKeepsNotice(t *testing.T) {
	part := convertRaven(t, &MessageConverter{DirectMedia: true, DisableViewOnce: true}, ravenContent("SlideMessageRavenImageContent", 0, true))
	if part.Content.MsgType != event.MsgNotice || !strings.Contains(part.Content.Body, "viewed once") {
		t.Fatalf("got %q %q", part.Content.MsgType, part.Content.Body)
	}
	if _, ok := ravenMarker(t, part); ok {
		t.Fatal("notice must not carry the raven marker")
	}
}

func TestRavenMarkerSkipsFailedUploads(t *testing.T) {
	notice := errorToNotice(context.Canceled, "raven image", nil)
	if got := markChatLoomRaven(notice, slidetypes.RavenViewModeViewOnce); got.Extra[ChatLoomRavenKey] != nil {
		t.Fatal("a failed upload notice must not carry the raven marker")
	}
	if markChatLoomRaven(nil, slidetypes.RavenViewModeViewOnce) != nil {
		t.Fatal("nil part must stay nil")
	}
}
