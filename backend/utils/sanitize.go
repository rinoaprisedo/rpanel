package utils

import "regexp"

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// StripTags removes HTML tags from a string as a basic XSS guard.
func StripTags(s string) string {
	return htmlTagRe.ReplaceAllString(s, "")
}
