package connector

import (
	"slices"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestNormalizeFacebookUsername(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"   ":                 "",
		"zuck":                "zuck",
		"  anna.kowalska.5 ":  "anna.kowalska.5",
		"@anna.k":             "anna.k",
		"100012345678901":     "",
		"123.456":             "",
		"anna kowalska":       "",
		"anna_k":              "",
		"zażółć":              "",
		"profile.php?id=1234": "",
		"a":                   "a",
		"x0000000000000000000000000000000000000000000000000000000000000000": "",
	}
	for in, want := range cases {
		if got := normalizeFacebookUsername(in); got != want {
			t.Errorf("normalizeFacebookUsername(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFacebookIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		info types.UserInfo
		want []string
	}{
		{"contact with username", &table.LSDeleteThenInsertContact{Id: 4, Name: "Anna", Username: "anna.k"}, []string{"facebook:anna.k"}},
		{"contact without username", &table.LSDeleteThenInsertContact{Id: 4, Name: "Anna"}, nil},
		{"secondary name is not a username", &table.LSDeleteThenInsertContact{Id: 4, Name: "Anna", SecondaryName: "kowalska"}, nil},
		{"numeric username", &table.LSDeleteThenInsertContact{Id: 4, Username: "100012345678901"}, nil},
		{"verify row has no username column", &table.LSVerifyContactRowExists{ContactId: 4, SecondaryName: "anna.k"}, nil},
		{"search result", &table.LSInsertSearchResult{ResultId: "4", DisplayName: "Anna"}, nil},
		{"nil contact", (*table.LSDeleteThenInsertContact)(nil), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := facebookIdentifiers(tt.info)
			if !slices.Equal(got, tt.want) || (got == nil) != (tt.want == nil) {
				t.Errorf("facebookIdentifiers() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestWrapUserInfoIdentifiers(t *testing.T) {
	m := &MetaClient{Main: &MetaConnector{Config: Config{DisplaynameTemplate: `{{or .DisplayName .Username "Unknown user"}}`}}}
	if err := m.Main.Config.PostProcess(); err != nil {
		t.Fatal(err)
	}

	info := m.wrapUserInfo(&table.LSDeleteThenInsertContact{Id: 4, Name: "Anna", Username: "anna.k"})
	if !slices.Equal(info.Identifiers, []string{"facebook:anna.k"}) {
		t.Errorf("Identifiers = %#v, want [facebook:anna.k]", info.Identifiers)
	}
	if info.Name == nil || *info.Name != "Anna" {
		t.Errorf("Name = %v, want Anna", info.Name)
	}
	if info.IsBot == nil || *info.IsBot {
		t.Errorf("IsBot = %v, want false", info.IsBot)
	}

	// Rows without a username must leave the stored identifiers alone (nil, not an empty slice),
	// otherwise a search result or a verify row would erase a username learned earlier.
	for _, row := range []types.UserInfo{
		&table.LSDeleteThenInsertContact{Id: 4, Name: "Anna"},
		&table.LSVerifyContactRowExists{ContactId: 4, Name: "Anna"},
		&table.LSInsertSearchResult{ResultId: "4", DisplayName: "Anna"},
	} {
		if got := m.wrapUserInfo(row).Identifiers; got != nil {
			t.Errorf("wrapUserInfo(%T).Identifiers = %#v, want nil", row, got)
		}
	}
}
