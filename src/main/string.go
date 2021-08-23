package main

func trimLongString(s string, limit int) string {
	if len(s) > limit {
		s = s[:limit]
	}
	return s
}
