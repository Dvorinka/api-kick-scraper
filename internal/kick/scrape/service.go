package scrape

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	reJSONLDScript = regexp.MustCompile(`(?is)<script[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	reMediaAttrs   = regexp.MustCompile(`(?is)data-media-id=["']([^"']*)["'][^>]*data-media-type=["']([^"']*)["'][^>]*data-media-url=["']([^"']*)["'][^>]*data-media-title=["']([^"']*)["'][^>]*data-media-views=["']([^"']*)["'][^>]*data-media-duration=["']([^"']*)["'][^>]*data-media-published=["']([^"']*)["']`)
)

type Service struct {
	httpClient *http.Client
	baseURL    string
}

func NewService(baseURL string) *Service {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = "https://kick.com"
	}
	trimmed = strings.TrimRight(trimmed, "/")

	return &Service{
		httpClient: &http.Client{Timeout: 12 * time.Second},
		baseURL:    trimmed,
	}
}

func (s *Service) GetChannel(ctx context.Context, input ChannelInput) (ChannelResult, error) {
	path, channel, err := normalizeChannelPath(input.ChannelURL, input.Channel)
	if err != nil {
		return ChannelResult{}, err
	}

	page, err := s.fetch(ctx, path)
	if err != nil {
		return ChannelResult{}, err
	}

	result := parseChannel(page)
	result.Channel = channel
	if result.DisplayName == "" {
		result.DisplayName = channel
	}
	if result.Channel == "" {
		result.Channel = channel
	}

	if input.IncludeMedia {
		result.Media = filterAndLimitMedia(parseMedia(page), "all", normalizeLimit(input.Limit))
	}
	return result, nil
}

func (s *Service) GetChannelMedia(ctx context.Context, input ChannelMediaInput) (ChannelMediaResult, error) {
	path, channel, err := normalizeChannelPath(input.ChannelURL, input.Channel)
	if err != nil {
		return ChannelMediaResult{}, err
	}

	page, err := s.fetch(ctx, path)
	if err != nil {
		return ChannelMediaResult{}, err
	}

	typeFilter := normalizeMediaType(input.Type)
	items := filterAndLimitMedia(parseMedia(page), typeFilter, normalizeLimit(input.Limit))
	return ChannelMediaResult{
		Channel: channel,
		Type:    typeFilter,
		Count:   len(items),
		Items:   items,
	}, nil
}

func (s *Service) IngestSignals(input SignalIngestInput) (SignalIngestResult, error) {
	channel := strings.TrimSpace(input.Channel)
	if channel == "" {
		return SignalIngestResult{}, errors.New("channel is required")
	}
	if len(input.Events) == 0 {
		return SignalIngestResult{}, errors.New("events cannot be empty")
	}
	if len(input.Events) > 1000 {
		return SignalIngestResult{}, errors.New("max 1000 events per request")
	}

	userSet := make(map[string]struct{}, len(input.Events))
	subscriberEvents := 0
	viewerTotal := 0
	viewerCountEvents := 0
	timestamps := make([]time.Time, 0, len(input.Events))

	for _, event := range input.Events {
		if user := strings.TrimSpace(event.User); user != "" {
			userSet[user] = struct{}{}
		}
		if event.IsSubscriber {
			subscriberEvents++
		}
		if event.ViewerCount > 0 {
			viewerTotal += event.ViewerCount
			viewerCountEvents++
		}
		if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(event.Timestamp)); err == nil {
			timestamps = append(timestamps, ts.UTC())
		}
	}

	messageRatePerMin := float64(len(input.Events))
	if len(timestamps) >= 2 {
		sort.Slice(timestamps, func(i, j int) bool { return timestamps[i].Before(timestamps[j]) })
		durationMinutes := timestamps[len(timestamps)-1].Sub(timestamps[0]).Minutes()
		if durationMinutes > 0 {
			messageRatePerMin = float64(len(input.Events)) / durationMinutes
		}
	}

	subscriberRatio := float64(subscriberEvents) / float64(len(input.Events))
	averageViewerCount := 0.0
	if viewerCountEvents > 0 {
		averageViewerCount = float64(viewerTotal) / float64(viewerCountEvents)
	}

	return SignalIngestResult{
		Channel:            channel,
		EventCount:         len(input.Events),
		UniqueUsers:        len(userSet),
		MessageRatePerMin:  roundTo2(messageRatePerMin),
		SubscriberRatio:    roundTo2(subscriberRatio),
		AverageViewerCount: roundTo2(averageViewerCount),
	}, nil
}

func normalizeChannelPath(channelURL, channel string) (path, handle string, err error) {
	raw := strings.TrimSpace(channelURL)
	if raw == "" {
		raw = strings.TrimSpace(channel)
	}
	if raw == "" {
		return "", "", errors.New("channel_url or channel is required")
	}

	if strings.Contains(raw, "://") {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil {
			return "", "", errors.New("invalid channel_url")
		}
		raw = parsed.Path
	}

	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "", "", errors.New("invalid channel path")
	}

	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		raw = parts[len(parts)-1]
	}
	if raw == "" {
		return "", "", errors.New("invalid channel path")
	}

	return "/" + raw, raw, nil
}

func (s *Service) fetch(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("User-Agent", "apitera-kick/1.0")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
	if err != nil {
		return "", fmt.Errorf("failed reading upstream body: %w", err)
	}
	return string(body), nil
}

func parseChannel(page string) ChannelResult {
	result := ChannelResult{}

	result.Channel = firstNonEmpty(
		findDataAttr(page, "channel-name"),
		findDataAttr(page, "channel"),
	)
	result.DisplayName = firstNonEmpty(
		findDataAttr(page, "display-name"),
		findDataAttr(page, "channel-display-name"),
	)
	result.Title = firstNonEmpty(
		findDataAttr(page, "stream-title"),
		findDataAttr(page, "title"),
	)
	result.Category = firstNonEmpty(
		findDataAttr(page, "category"),
		findDataAttr(page, "stream-category"),
	)
	result.IsLive = parseBool(firstNonEmpty(findDataAttr(page, "live"), findDataAttr(page, "is-live")))
	result.ViewerCount = parseInt(findDataAttr(page, "viewers"))
	result.FollowerCount = parseInt(findDataAttr(page, "followers"))
	result.ThumbnailURL = firstNonEmpty(findDataAttr(page, "thumbnail-url"), findDataAttr(page, "avatar-url"))
	result.StartedAt = findDataAttr(page, "started-at")

	nodes := extractJSONLDNodes(page)
	for _, node := range nodes {
		if isType(node, "VideoObject") || isType(node, "BroadcastEvent") {
			if result.Title == "" {
				result.Title = asString(node["name"])
			}
			if result.ThumbnailURL == "" {
				result.ThumbnailURL = asString(node["thumbnailUrl"])
			}
			if result.StartedAt == "" {
				result.StartedAt = asString(node["startDate"])
			}
		}
		if isType(node, "Person") {
			if result.DisplayName == "" {
				result.DisplayName = asString(node["name"])
			}
		}
	}

	return result
}

func parseMedia(page string) []MediaItem {
	matches := reMediaAttrs.FindAllStringSubmatch(page, -1)
	if len(matches) == 0 {
		return nil
	}

	items := make([]MediaItem, 0, len(matches))
	for _, m := range matches {
		if len(m) < 8 {
			continue
		}
		item := MediaItem{
			ID:          strings.TrimSpace(m[1]),
			Type:        normalizeMediaType(strings.TrimSpace(m[2])),
			URL:         html.UnescapeString(strings.TrimSpace(m[3])),
			Title:       html.UnescapeString(strings.TrimSpace(m[4])),
			ViewCount:   parseInt(strings.TrimSpace(m[5])),
			Duration:    strings.TrimSpace(m[6]),
			PublishedAt: strings.TrimSpace(m[7]),
		}
		if item.Title == "" {
			item.Title = "Untitled"
		}
		items = append(items, item)
	}
	return items
}

func filterAndLimitMedia(items []MediaItem, mediaType string, limit int) []MediaItem {
	if len(items) == 0 {
		return nil
	}
	if mediaType == "" {
		mediaType = "all"
	}

	filtered := make([]MediaItem, 0, minInt(limit, len(items)))
	for _, item := range items {
		if mediaType != "all" && item.Type != mediaType {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func extractJSONLDNodes(page string) []map[string]any {
	blocks := reJSONLDScript.FindAllStringSubmatch(page, -1)
	if len(blocks) == 0 {
		return nil
	}

	nodes := make([]map[string]any, 0, len(blocks)*2)
	for _, block := range blocks {
		raw := strings.TrimSpace(block[1])
		if raw == "" {
			continue
		}
		raw = html.UnescapeString(raw)

		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			continue
		}
		collectMapNodes(decoded, &nodes)
	}
	return nodes
}

func collectMapNodes(value any, out *[]map[string]any) {
	switch v := value.(type) {
	case map[string]any:
		*out = append(*out, v)
		for _, inner := range v {
			collectMapNodes(inner, out)
		}
	case []any:
		for _, inner := range v {
			collectMapNodes(inner, out)
		}
	}
}

func findDataAttr(page, name string) string {
	pattern := fmt.Sprintf(`(?is)data-%s=["']([^"']+)["']`, regexp.QuoteMeta(name))
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(page)
	if len(match) < 2 {
		return ""
	}
	return html.UnescapeString(strings.TrimSpace(match[1]))
}

func isType(node map[string]any, want string) bool {
	raw, ok := node["@type"]
	if !ok {
		return false
	}
	wantLower := strings.ToLower(strings.TrimSpace(want))
	switch t := raw.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(t)) == wantLower
	case []any:
		for _, item := range t {
			if strings.ToLower(strings.TrimSpace(asString(item))) == wantLower {
				return true
			}
		}
	}
	return false
}

func asString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", v), "0"), ".")
	case int:
		return fmt.Sprintf("%d", v)
	case map[string]any:
		if text := asString(v["name"]); text != "" {
			return text
		}
		if text := asString(v["url"]); text != "" {
			return text
		}
	case []any:
		for _, item := range v {
			if text := asString(item); text != "" {
				return text
			}
		}
	}
	return ""
}

func normalizeMediaType(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "vod", "vods", "video":
		return "vod"
	case "clip", "clips":
		return "clip"
	case "all", "":
		return "all"
	default:
		return normalized
	}
}

func parseInt(raw string) int {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return 0
	}
	value = strings.ReplaceAll(value, ",", "")
	value = strings.ReplaceAll(value, "+", "")
	value = strings.ReplaceAll(value, "viewers", "")
	value = strings.ReplaceAll(value, "followers", "")
	value = strings.ReplaceAll(value, "views", "")
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "k") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(value, "k"), 64)
		if err == nil {
			return int(f * 1000)
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func parseBool(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return value == "1" || value == "true" || value == "yes" || value == "live" || value == "online"
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func roundTo2(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
