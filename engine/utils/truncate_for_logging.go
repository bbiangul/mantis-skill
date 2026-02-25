package utils

func TruncateForLogging(s string) string {
	const maxLength = 1000
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}
