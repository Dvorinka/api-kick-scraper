package scrape

type ChannelInput struct {
	ChannelURL   string `json:"channel_url,omitempty"`
	Channel      string `json:"channel,omitempty"`
	IncludeMedia bool   `json:"include_media,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

type ChannelMediaInput struct {
	ChannelURL string `json:"channel_url,omitempty"`
	Channel    string `json:"channel,omitempty"`
	Type       string `json:"type,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type MediaItem struct {
	ID          string `json:"id,omitempty"`
	Type        string `json:"type,omitempty"`
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	ViewCount   int    `json:"view_count,omitempty"`
	Duration    string `json:"duration,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type ChannelResult struct {
	Channel       string      `json:"channel"`
	DisplayName   string      `json:"display_name,omitempty"`
	Title         string      `json:"title,omitempty"`
	Category      string      `json:"category,omitempty"`
	IsLive        bool        `json:"is_live"`
	ViewerCount   int         `json:"viewer_count,omitempty"`
	FollowerCount int         `json:"follower_count,omitempty"`
	ThumbnailURL  string      `json:"thumbnail_url,omitempty"`
	StartedAt     string      `json:"started_at,omitempty"`
	Media         []MediaItem `json:"media,omitempty"`
}

type ChannelMediaResult struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Count   int         `json:"count"`
	Items   []MediaItem `json:"items"`
}

type SignalEvent struct {
	User         string `json:"user"`
	Message      string `json:"message"`
	Timestamp    string `json:"timestamp"`
	ViewerCount  int    `json:"viewer_count,omitempty"`
	IsSubscriber bool   `json:"is_subscriber,omitempty"`
}

type SignalIngestInput struct {
	Channel string        `json:"channel"`
	Events  []SignalEvent `json:"events"`
}

type SignalIngestResult struct {
	Channel            string  `json:"channel"`
	EventCount         int     `json:"event_count"`
	UniqueUsers        int     `json:"unique_users"`
	MessageRatePerMin  float64 `json:"message_rate_per_min"`
	SubscriberRatio    float64 `json:"subscriber_ratio"`
	AverageViewerCount float64 `json:"average_viewer_count"`
}
