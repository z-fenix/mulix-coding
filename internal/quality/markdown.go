package quality

import "strings"

// fenceStart preserves the delimiter length so nested example fences cannot
// close an outer fence or expose quoted task headings.
func fenceStart(line string) string {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "```") && !strings.HasPrefix(t, "~~~") {
		return ""
	}
	n := 3
	for n < len(t) && t[n] == t[0] {
		n++
	}
	return t[:n]
}
func fenceEnd(line, marker string) bool {
	t := strings.TrimSpace(line)
	return len(t) >= len(marker) && strings.Trim(t, string(marker[0])) == ""
}
func proseOnly(md string) string {
	var out []string
	marker := ""
	for _, line := range strings.Split(md, "\n") {
		if marker != "" {
			if fenceEnd(line, marker) {
				marker = ""
			}
			continue
		}
		if opening := fenceStart(line); opening != "" {
			marker = opening
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
