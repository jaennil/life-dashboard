package handlers

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	screenTimeKindApp     = "app"
	screenTimeKindWebsite = "website"
	screenTimeMaxSeconds  = 86400
)

// The iOS 26 Shortcuts action "Get App & Website Activity" serializes its result
// as one line per entry, sorted by descending duration:
//
//	Instagram (6h 55m)
//	zvonite-solu.com (47m)
//	Yandex Books (38s)
//	ru.vtb24.mobilebanking.iphone (12s)
//
// Names are localized display names (bundle identifiers only when iOS failed to
// resolve one) and may contain non-breaking spaces.
var screenTimeLineRe = regexp.MustCompile(`^(.*\S)\s*\(([^()]+)\)$`)

var screenTimeDurationPartRe = regexp.MustCompile(`(\d+)\s*([\p{L}]+)`)

type screenTimeItem struct {
	Kind         string
	ItemKey      string
	DisplayName  string
	Seconds      int
	KindInferred bool
	Clamped      bool
}

type screenTimeParseResult struct {
	Items    []screenTimeItem
	Unparsed []string
}

// parseScreenTimeBlob turns one payload field into items. A non-empty kind means
// the field came from an apps-only or websites-only action and the classification
// is authoritative; an empty kind means the field holds the combined list and the
// app/website split has to be inferred per line.
func parseScreenTimeBlob(blob, kind string) screenTimeParseResult {
	result := screenTimeParseResult{}
	seen := make(map[string]int)

	for rawLine := range strings.SplitSeq(normalizeScreenTimeText(blob), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		match := screenTimeLineRe.FindStringSubmatch(line)
		if match == nil {
			result.Unparsed = append(result.Unparsed, line)
			continue
		}

		name := strings.TrimSpace(match[1])
		seconds, ok := parseScreenTimeDuration(match[2])
		if !ok || name == "" {
			result.Unparsed = append(result.Unparsed, line)
			continue
		}

		item := screenTimeItem{
			Kind:        kind,
			DisplayName: truncateScreenTimeText(name),
			Seconds:     seconds,
		}
		if item.Kind == "" {
			item.KindInferred = true
			item.Kind = screenTimeKindApp
			if looksLikeHostname(name) {
				item.Kind = screenTimeKindWebsite
			}
		}
		if item.Seconds > screenTimeMaxSeconds {
			item.Seconds = screenTimeMaxSeconds
			item.Clamped = true
		}
		item.ItemKey = screenTimeItemKey(name)

		// The same name can legitimately appear twice when the caller merges
		// fields; keep the larger value instead of letting the upsert race.
		dedupeKey := item.Kind + "\x00" + item.ItemKey
		if idx, exists := seen[dedupeKey]; exists {
			if item.Seconds > result.Items[idx].Seconds {
				result.Items[idx] = item
			}
			continue
		}
		seen[dedupeKey] = len(result.Items)
		result.Items = append(result.Items, item)
	}

	return result
}

// parseScreenTimeDuration reads "6h 55m", "47m", "52s" or "1h 2m 3s" into seconds.
func parseScreenTimeDuration(value string) (int, bool) {
	matches := screenTimeDurationPartRe.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return 0, false
	}

	total := 0
	for _, match := range matches {
		amount, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, false
		}
		unit, ok := screenTimeUnitSeconds(match[2])
		if !ok {
			return 0, false
		}
		total += amount * unit
	}
	return total, true
}

func screenTimeUnitSeconds(unit string) (int, bool) {
	switch strings.ToLower(unit) {
	case "h", "hr", "hrs", "hour", "hours", "ч", "час", "часа", "часов":
		return 3600, true
	case "m", "min", "mins", "minute", "minutes", "м", "мин", "минута", "минуты", "минут":
		return 60, true
	case "s", "sec", "secs", "second", "seconds", "с", "сек", "секунда", "секунды", "секунд":
		return 1, true
	}
	return 0, false
}

// looksLikeHostname separates website entries from app names in the combined
// list. The discriminator is the last label: a hostname ends in a public suffix
// ("account.xiaomi.com", "ya.ru"), whereas an unresolved bundle identifier ends
// in a product name ("ru.vtb24.mobilebanking.iphone").
func looksLikeHostname(name string) bool {
	if strings.ContainsAny(name, " \t/@:") || !strings.Contains(name, ".") {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 || slices.Contains(labels, "") {
		return false
	}

	suffix := strings.ToLower(labels[len(labels)-1])
	// Every two-letter suffix is a country-code TLD; no Apple product name is
	// two letters, so this covers ru/me/io/co/uk without enumerating them.
	if len(suffix) == 2 {
		return true
	}
	return screenTimeKnownSuffixes[suffix]
}

var screenTimeKnownSuffixes = map[string]bool{
	"com": true, "net": true, "org": true, "info": true, "biz": true,
	"gov": true, "edu": true, "int": true, "mil": true, "pro": true,
	"app": true, "dev": true, "cloud": true, "online": true, "site": true,
	"store": true, "shop": true, "tech": true, "space": true, "website": true,
	"xyz": true, "top": true, "club": true, "live": true, "news": true,
	"blog": true, "wiki": true, "farm": true, "fun": true, "life": true,
	"link": true, "media": true, "moscow": true, "name": true, "network": true,
	"one": true, "press": true, "pub": true, "rent": true, "rest": true,
	"run": true, "sale": true, "school": true, "science": true, "services": true,
	"social": true, "software": true, "solutions": true, "studio": true,
	"today": true, "tools": true, "video": true, "watch": true, "work": true,
	"world": true, "zone": true, "agency": true, "art": true, "bar": true,
	"best": true, "bid": true, "cash": true, "center": true, "chat": true,
	"city": true, "company": true, "digital": true, "email": true, "expert": true,
	"finance": true, "games": true, "group": true, "guru": true, "host": true,
	"house": true,
}

// normalizeScreenTimeText normalizes line endings and the exotic spaces iOS puts
// inside some display names (e.g. "Yandex\u00a0Maps").
func normalizeScreenTimeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for _, space := range []string{"\u00a0", "\u2009", "\u202f", "\u2007"} {
		text = strings.ReplaceAll(text, space, " ")
	}
	return text
}

func screenTimeItemKey(name string) string {
	return truncateScreenTimeText(strings.ToLower(strings.Join(strings.Fields(name), " ")))
}

// truncateScreenTimeText keeps values inside the VARCHAR(255) columns without
// splitting a multi-byte rune, which Cyrillic app names make a real risk.
func truncateScreenTimeText(value string) string {
	const limit = 255
	if len(value) <= limit {
		return value
	}
	runes := []rune(value)
	for len(string(runes)) > limit {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}
