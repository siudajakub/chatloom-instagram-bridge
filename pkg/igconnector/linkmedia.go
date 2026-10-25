package igconnector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/responses"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
	"maunium.net/go/mautrix/bridgev2"
)

var linkMediaGate = make(chan struct{}, 2)

type linkMediaRequest struct {
	URL      string `json:"url"`
	Index    int    `json:"index"`
	Download bool   `json:"download"`
}
type linkMediaTarget struct {
	ID        string
	Shortcode string
	Story     bool
	Highlight bool
}

var linkCode = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
var linkNumber = regexp.MustCompile(`^[0-9]{1,30}$`)

func parseLinkMedia(raw string) (linkMediaTarget, error) {
	fail := fmt.Errorf("unsupported Instagram permalink")
	if len(raw) > 4096 {
		return linkMediaTarget{}, fail
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return linkMediaTarget{}, fail
	}
	switch strings.ToLower(u.Hostname()) {
	case "instagram.com", "www.instagram.com", "m.instagram.com", "instagr.am":
	default:
		return linkMediaTarget{}, fail
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 3 && parts[1] == "reel" {
		parts = parts[1:]
	}
	if len(parts) == 3 && parts[0] == "stories" && linkNumber.MatchString(parts[2]) {
		return linkMediaTarget{ID: parts[2], Story: true, Highlight: parts[1] == "highlights"}, nil
	}
	if len(parts) != 2 || !linkCode.MatchString(parts[1]) {
		return linkMediaTarget{}, fail
	}
	switch parts[0] {
	case "p", "reel", "reels", "tv":
	default:
		return linkMediaTarget{}, fail
	}
	n := new(big.Int)
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	for _, c := range parts[1] {
		n.Lsh(n, 6)
		n.Add(n, big.NewInt(int64(strings.IndexRune(alphabet, c))))
	}
	return linkMediaTarget{ID: n.String(), Shortcode: parts[1]}, nil
}
func (ic *IGClient) linkMediaItems(ctx context.Context, t linkMediaTarget) ([]*responses.Items, error) {
	if !t.Story {
		res, err := ic.Client.FetchMedia(ctx, t.ID, t.Shortcode)
		if err != nil {
			return nil, err
		}
		if len(res.Items) == 0 {
			return nil, fmt.Errorf("media unavailable")
		}
		item := res.Items[0]
		if len(item.CarouselMedia) > 0 {
			return item.CarouselMedia, nil
		}
		return []*responses.Items{item}, nil
	}
	if t.Highlight {
		res, err := ic.Client.FetchReel(ctx, []string{"highlight:" + t.ID}, "")
		if err != nil {
			return nil, err
		}
		var items []*responses.Items
		reels := res.ReelsMedia
		for _, reel := range res.Reels {
			reels = append(reels, reel)
		}
		for _, reel := range reels {
			for _, item := range reel.Items {
				items = append(items, &item.Items)
			}
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("highlight unavailable")
		}
		return items, nil
	}
	// Story links carry no reel (user) ID, so the reel lookup may be refused;
	// the media lookup below is still tried in that case.
	res, err := ic.Client.FetchReel(ctx, nil, t.ID)
	if err == nil {
		reels := res.ReelsMedia
		for _, reel := range res.Reels {
			reels = append(reels, reel)
		}
		for _, reel := range reels {
			for _, item := range reel.Items {
				if item.Pk.String() == t.ID {
					return []*responses.Items{&item.Items}, nil
				}
			}
		}
	}
	fallback, fallbackErr := ic.Client.FetchMedia(ctx, t.ID, "")
	if fallbackErr == nil && len(fallback.Items) > 0 {
		return fallback.Items, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("story unavailable")
}
func (ic *IGConnector) registerLinkMedia() {
	matrix, ok := ic.Bridge.Matrix.(bridgev2.MatrixConnectorWithProvisioning)
	if !ok {
		return
	}
	prov := matrix.GetProvisioning()
	prov.GetRouter().HandleFunc("POST /v3/chatloom/link_media", func(w http.ResponseWriter, r *http.Request) {
		select {
		case linkMediaGate <- struct{}{}:
			defer func() { <-linkMediaGate }()
		default:
			http.Error(w, "busy", 429)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		// The existing provisioning middleware authenticates the Matrix user.
		user := prov.GetUser(r)
		if user == nil {
			http.Error(w, "authentication required", 401)
			return
		}
		var client *IGClient
		for _, login := range user.GetUserLogins() {
			if c, ok := login.Client.(*IGClient); ok && c.Client != nil {
				if client != nil {
					http.Error(w, "ambiguous Instagram login", 409)
					return
				}
				client = c
			}
		}
		if client == nil {
			http.Error(w, "Instagram login required", 403)
			return
		}
		var req linkMediaRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		dec.DisallowUnknownFields()
		if dec.Decode(&req) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		target, err := parseLinkMedia(req.URL)
		if err != nil {
			http.Error(w, "unsupported permalink", 400)
			return
		}
		items, err := client.linkMediaItems(r.Context(), target)
		if err != nil {
			w.Header().Set("X-Chatloom-Error", "fetch_failed")
			if strings.Contains(err.Error(), "highlight unavailable") {
				w.Header().Set("X-Chatloom-Error", "empty_highlight")
			}
			var syntaxError *json.SyntaxError
			if errors.As(err, &syntaxError) {
				w.Header().Set("X-Chatloom-Error", "unsupported_json")
			}

			var typeError *json.UnmarshalTypeError
			if errors.As(err, &typeError) {
				field := typeError.Field
				if i := strings.LastIndex(field, "."); i >= 0 {
					field = field[i+1:]
				}
				if regexp.MustCompile(`^[A-Za-z_]{1,64}$`).MatchString(field) {
					w.Header().Set("X-Chatloom-Error", "unsupported_field_"+field)
				}
			}

			http.Error(w, "Instagram media unavailable", 502)
			return
		}
		if len(items) > 128 || req.Index < 0 || req.Index >= len(items) {
			http.Error(w, "invalid media index", 400)
			return
		}
		type asset struct {
			Type   string `json:"type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		}
		assets := make([]asset, len(items))
		urls := make([]string, len(items))
		for i, item := range items {
			a := asset{Type: "image"}
			for _, v := range item.VideoVersions {
				if v.URL != "" && (urls[i] == "" || v.Width*v.Height > a.Width*a.Height) {
					a = asset{Type: "video", Width: v.Width, Height: v.Height}
					urls[i] = v.URL
				}
			}
			if urls[i] == "" {
				for _, img := range item.ImageVersions2.Candidates {
					if img.URL != "" && (urls[i] == "" || img.Width*img.Height > a.Width*a.Height) {
						a = asset{Type: "image", Width: img.Width, Height: img.Height}
						urls[i] = img.URL
					}
				}
			}
			if urls[i] == "" {
				w.Header().Set("X-Chatloom-Error", "missing_asset_url")
				http.Error(w, "media unavailable", 502)
				return
			}
			assets[i] = a
		}
		w.Header().Set("Cache-Control", "no-store")
		if !req.Download {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"assets": assets})
			return
		}
		// Only server-returned media URLs are used, never an arbitrary caller URL.
		mime := "video/mp4"
		if assets[req.Index].Type == "image" {
			mime = "image/jpeg"
		}
		size, reader, err := mediadl.DownloadMedia(r.Context(), mime, urls[req.Index], ic.MsgConv.MaxFileSize)
		if err != nil {
			http.Error(w, "download failed", 502)
			return
		}
		defer reader.Close()
		if size < 1 || size > 64*1024*1024 {
			http.Error(w, "media too large", 413)
			return
		}
		data, err := io.ReadAll(io.LimitReader(reader, size+1))
		if err != nil || int64(len(data)) != size {
			http.Error(w, "incomplete media", 502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"mime": mime, "data": base64.StdEncoding.EncodeToString(data)})
	})
}
