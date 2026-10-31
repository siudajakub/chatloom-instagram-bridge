package connector

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

func (m *MetaClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	if ghost.Name == "" {
		contactID := metaid.ParseUserID(ghost.ID)
		resp, err := m.Client.ExecuteTasks(ctx, &socket.GetContactsFullTask{
			ContactID: contactID,
		})
		if err != nil {
			return nil, err
		}
		for _, info := range resp.LSDeleteThenInsertIGContactInfo {
			err := m.Main.DB.PutFBIDForIGUser(ctx, info.IgId, info.ContactId)
			if err != nil {
				zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to save FBID for IG user")
			}
		}
		if len(resp.LSDeleteThenInsertContact) > 0 {
			return m.wrapUserInfo(resp.LSDeleteThenInsertContact[0]), nil
		} else {
			return nil, fmt.Errorf("user info not found via GetContactsFullTask")
		}
	}
	return nil, nil
}

const (
	MetaAIInstagramID = 656175869434325
	MetaAIMessengerID = 156025504001094
)

func (m *MetaClient) wrapUserInfo(info types.UserInfo) *bridgev2.UserInfo {
	return &bridgev2.UserInfo{
		Name: ptr.Ptr(m.Main.Config.FormatDisplayname(DisplaynameParams{
			DisplayName: info.GetName(),
			Username:    info.GetUsername(),
			ID:          info.GetFBID(),
		})),
		Avatar:      wrapAvatar(info.GetAvatarURL()),
		IsBot:       ptr.Ptr(info.GetFBID() == MetaAIInstagramID || info.GetFBID() == MetaAIMessengerID), // TODO do this in a less hardcoded way?
		Identifiers: facebookIdentifiers(info),
	}
}

// facebookIdentifiers returns the ghost's network identifiers (bridgev2 writes them to the profile as
// com.beeper.bridge.identifiers): ["facebook:<username>"] when the contact row carries a Facebook
// username, nil otherwise.
//
// Only LSDeleteThenInsertContact has a dedicated username column (index 43). GetUsername() returns
// SecondaryName, which is not reliably a username (it can hold a second name), so it is not used.
// A nil result leaves previously stored identifiers untouched: search results and
// LSVerifyContactRowExists rows carry no username and must not wipe one learned from a full contact
// row.
func facebookIdentifiers(info types.UserInfo) []string {
	contact, ok := info.(*table.LSDeleteThenInsertContact)
	if !ok || contact == nil {
		return nil
	}
	username := normalizeFacebookUsername(contact.Username)
	if username == "" {
		return nil
	}
	return []string{"facebook:" + username}
}

// normalizeFacebookUsername returns username if it looks like a Facebook vanity username (letters,
// digits and periods, at least one letter), or "" for empty values, numeric IDs and anything else.
func normalizeFacebookUsername(username string) string {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" || len(username) > 64 {
		return ""
	}
	hasLetter := false
	for _, r := range username {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9', r == '.':
		default:
			return ""
		}
	}
	if !hasLetter {
		return ""
	}
	return username
}

func wrapAvatar(avatarURL string) *bridgev2.Avatar {
	if avatarURL == "" {
		return &bridgev2.Avatar{Remove: true}
	}
	parsedURL, _ := url.Parse(avatarURL)
	avatarID := path.Base(parsedURL.Path)
	return &bridgev2.Avatar{
		ID: networkid.AvatarID(avatarID),
		Get: func(ctx context.Context) ([]byte, error) {
			return mediadl.DownloadAvatar(ctx, avatarURL)
		},
	}
}
