package syslog

// Format selects the syslog message format. Over TCP and TLS every format is
// sent with octet-counting framing.
type Format int

const (
	// FormatRFC5424 is the syslog protocol of RFC 5424, as produced by
	// rsyslog's RSYSLOG_SyslogRFC5424Format and RSYSLOG_SyslogProtocol23Format
	// templates. This is the default:
	//
	//	<PRI>1 2026-01-02T15:04:05.000000+13:00 HOSTNAME APP-NAME PROCID - - MSG
	FormatRFC5424 Format = iota

	// FormatForward is rsyslog's RSYSLOG_ForwardFormat: the BSD layout with a
	// high-precision RFC 3339 timestamp, for receivers without RFC 5424
	// support:
	//
	//	<PRI>2026-01-02T15:04:05.000000+13:00 HOSTNAME APP-NAME[PROCID]: MSG
	FormatForward

	// FormatTraditionalForward is rsyslog's RSYSLOG_TraditionalForwardFormat,
	// the classic BSD syslog format of RFC 3164 with a low-precision local
	// timestamp, for older syslog daemons and appliances:
	//
	//	<PRI>Jan  2 15:04:05 HOSTNAME APP-NAME[PROCID]: MSG
	FormatTraditionalForward
)

// FormatSyslogProtocol23 matches rsyslog's RSYSLOG_SyslogProtocol23Format,
// named after draft-ietf-syslog-protocol-23, the final draft of RFC 5424. It
// is an alias for FormatRFC5424: the header is identical, and the template's
// trailing newline is omitted because the transport separates messages (one
// per UDP datagram, octet-counting framing over TCP and TLS).
const FormatSyslogProtocol23 = FormatRFC5424
