package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// SearchResult holds a single web search result.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// httpClient is shared, has a 15s timeout.
var httpClient = &http.Client{Timeout: 15 * time.Second}

// FetchURL performs a GET request and returns the response body as bytes.
// Limits response to 512KB to avoid memory bloat.
func FetchURL(rawURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	req.Header.Set("User-Agent", "NeXuS-CLI/1.0 (char-gen; +https://nexusnet.network)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	limited := io.LimitReader(resp.Body, 512*1024) // 512KB cap
	return io.ReadAll(limited)
}

// StripHTML removes HTML tags and collapses whitespace.
// Returns plain text suitable for LLM context.
func StripHTML(htmlBytes []byte) string {
	s := string(htmlBytes)
	// Remove script/style blocks entirely
	for _, tag := range []string{"script", "style", "head", "nav", "footer"} {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		s = re.ReplaceAllString(s, " ")
	}
	// Strip remaining tags
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, " ")
	// Decode common HTML entities
	s = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&nbsp;", " ",
	).Replace(s)
	// Collapse whitespace
	s = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(s), " ")
	// Cap at 4000 chars for LLM context
	if utf8.RuneCountInString(s) > 4000 {
		runes := []rune(s)
		s = string(runes[:4000]) + "..."
	}
	return s
}

// SearchWeb queries DuckDuckGo HTML and returns up to 5 results.
// No API key required.
func SearchWeb(query string) ([]SearchResult, error) {
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	data, err := FetchURL(searchURL)
	if err != nil {
		return nil, err
	}
	return parseDDGResults(string(data)), nil
}

// parseDDGResults extracts results from DuckDuckGo HTML response.
func parseDDGResults(html string) []SearchResult {
	var results []SearchResult

	// Match result blocks: <a class="result__a" href="...">title</a>
	titleRe := regexp.MustCompile(`class="result__a"[^>]*href="([^"]+)"[^>]*>([^<]+)</a>`)
	snippetRe := regexp.MustCompile(`class="result__snippet"[^>]*>([^<]+)`)

	titleMatches := titleRe.FindAllStringSubmatch(html, 10)
	snippetMatches := snippetRe.FindAllStringSubmatch(html, 10)

	for i, tm := range titleMatches {
		if i >= 5 {
			break
		}
		r := SearchResult{
			URL:   strings.TrimSpace(tm[1]),
			Title: strings.TrimSpace(tm[2]),
		}
		if i < len(snippetMatches) {
			r.Snippet = strings.TrimSpace(snippetMatches[i][1])
		}
		results = append(results, r)
	}
	return results
}

// FetchGist fetches a GitHub Gist and returns its raw text content.
// Accepts both gist.github.com/user/id and raw URLs.
// For normal gist URLs, converts to raw automatically.
func FetchGist(gistURL string) (string, error) {
	// Convert web URL to raw API if needed
	// https://gist.github.com/user/abc123 → https://api.github.com/gists/abc123
	if strings.Contains(gistURL, "gist.github.com") && !strings.Contains(gistURL, "raw") {
		parts := strings.Split(strings.TrimRight(gistURL, "/"), "/")
		gistID := parts[len(parts)-1]
		// Fetch via API to get raw_url
		apiURL := "https://api.github.com/gists/" + gistID
		data, err := FetchURL(apiURL)
		if err != nil {
			return "", fmt.Errorf("gist API fetch failed: %w", err)
		}
		// Extract first raw_url from JSON (simple regex, no full unmarshal needed)
		rawRe := regexp.MustCompile(`"raw_url"\s*:\s*"([^"]+)"`)
		m := rawRe.FindStringSubmatch(string(data))
		if m == nil {
			return "", fmt.Errorf("could not find raw_url in gist API response")
		}
		gistURL = m[1]
	}
	data, err := FetchURL(gistURL)
	if err != nil {
		return "", err
	}
	content := string(data)
	// Cap at 8000 chars for lore entries
	if utf8.RuneCountInString(content) > 8000 {
		runes := []rune(content)
		content = string(runes[:8000]) + "\n[...truncated at 8000 chars]"
	}
	return content, nil
}

// EstimateTokens gives a rough token count (1 token ≈ 4 chars).
func EstimateTokens(s string) int {
	return utf8.RuneCountInString(s) / 4
}

// TrimMessagesToBudget removes oldest messages (keeping system prompt at [0])
// until total estimated tokens fit within budget.
func TrimMessagesToBudget(messages []Message, budgetTokens int) []Message {
	if len(messages) == 0 {
		return messages
	}
	for {
		total := 0
		for _, m := range messages {
			total += EstimateTokens(m.Content)
		}
		if total <= budgetTokens || len(messages) <= 2 {
			break
		}
		// Remove second message (keep index 0 = system prompt)
		messages = append(messages[:1], messages[2:]...)
	}
	return messages
}
