package adapters

import "github.com/dancampari/harness/internal/sensors"

func finding(dim sensors.Dimension, severity sensors.Severity, file string, line int, rule, message string) sensors.Finding {
	f := sensors.Finding{
		Dimension: dim,
		Severity:  severity,
		File:      file,
		Line:      line,
		Rule:      rule,
		Message:   message,
	}
	f.Fingerprint = sensors.Fingerprint(dim, file, rule, message)
	return f
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}
