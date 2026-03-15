package scrape

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetChannel(t *testing.T) {
	html := `
	<html><head>
	<script type="application/ld+json">{"@context":"https://schema.org","@type":"VideoObject","name":"Live coding","thumbnailUrl":"https://cdn.example.com/thumb.jpg","startDate":"2026-02-24T12:00:00Z"}</script>
	</head><body>
	<div data-channel-name="apitera" data-display-name="Apitera Live" data-title="Go API deep dive" data-category="Programming" data-live="true" data-viewers="1842" data-followers="91200" data-thumbnail-url="https://cdn.example.com/channel.jpg" data-started-at="2026-02-24T12:00:00Z"></div>
	<div data-media-id="v1" data-media-type="vod" data-media-url="https://kick.com/video/1" data-media-title="Yesterday stream" data-media-views="12000" data-media-duration="02:10:00" data-media-published="2026-02-23T10:00:00Z"></div>
	<div data-media-id="c1" data-media-type="clip" data-media-url="https://kick.com/clip/1" data-media-title="Big moment" data-media-views="2500" data-media-duration="00:00:45" data-media-published="2026-02-24T12:10:00Z"></div>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apitera" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	svc := NewService(server.URL)
	result, err := svc.GetChannel(context.Background(), ChannelInput{
		ChannelURL:   "https://kick.com/apitera",
		IncludeMedia: true,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("get channel failed: %v", err)
	}
	if result.Channel != "apitera" {
		t.Fatalf("unexpected channel: %s", result.Channel)
	}
	if !result.IsLive {
		t.Fatalf("expected live channel")
	}
	if result.ViewerCount < 1000 {
		t.Fatalf("expected viewer count parsed")
	}
	if len(result.Media) != 2 {
		t.Fatalf("expected 2 media entries, got %d", len(result.Media))
	}
}

func TestGetChannelMediaFilter(t *testing.T) {
	html := `<div data-media-id="v1" data-media-type="vod" data-media-url="https://kick.com/video/1" data-media-title="VOD 1" data-media-views="10" data-media-duration="01:00" data-media-published="2026-02-24T10:00:00Z"></div>
<div data-media-id="c1" data-media-type="clip" data-media-url="https://kick.com/clip/1" data-media-title="Clip 1" data-media-views="20" data-media-duration="00:30" data-media-published="2026-02-24T10:01:00Z"></div>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	svc := NewService(server.URL)
	result, err := svc.GetChannelMedia(context.Background(), ChannelMediaInput{Channel: "apitera", Type: "clip", Limit: 5})
	if err != nil {
		t.Fatalf("get channel media failed: %v", err)
	}
	if result.Count != 1 {
		t.Fatalf("expected 1 clip, got %d", result.Count)
	}
	if result.Items[0].Type != "clip" {
		t.Fatalf("expected clip type")
	}
}

func TestIngestSignals(t *testing.T) {
	svc := NewService("https://kick.com")
	result, err := svc.IngestSignals(SignalIngestInput{
		Channel: "apitera",
		Events: []SignalEvent{
			{User: "u1", Message: "hello", Timestamp: "2026-02-24T12:00:00Z", ViewerCount: 100, IsSubscriber: true},
			{User: "u2", Message: "nice", Timestamp: "2026-02-24T12:00:30Z", ViewerCount: 102, IsSubscriber: false},
			{User: "u1", Message: "again", Timestamp: "2026-02-24T12:01:00Z", ViewerCount: 101, IsSubscriber: true},
		},
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if result.EventCount != 3 {
		t.Fatalf("unexpected event count: %d", result.EventCount)
	}
	if result.UniqueUsers != 2 {
		t.Fatalf("unexpected unique users: %d", result.UniqueUsers)
	}
	if result.SubscriberRatio <= 0.5 {
		t.Fatalf("expected subscriber ratio > 0.5")
	}
}
