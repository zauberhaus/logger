// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

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
