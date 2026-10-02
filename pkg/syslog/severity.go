package syslog

// Severity is a syslog severity code (RFC 5424, section 6.2.1).
type Severity int

const (
	SeverityEmergency Severity = iota
	SeverityAlert
	SeverityCritical
	SeverityError
	SeverityWarning
	SeverityNotice
	SeverityInfo
	SeverityDebug
)
