// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

// cspell:words UUCP
package syslog

// Facility is a syslog facility code (RFC 5424, section 6.2.1).
type Facility int

const (
	FacilityKern Facility = iota
	FacilityUser
	FacilityMail
	FacilityDaemon
	FacilityAuth
	FacilitySyslog
	FacilityLPR
	FacilityNews
	FacilityUUCP
	FacilityCron
	FacilityAuthPriv
	FacilityFTP
)

// Local use facilities.
const (
	FacilityLocal0 Facility = 16 + iota
	FacilityLocal1
	FacilityLocal2
	FacilityLocal3
	FacilityLocal4
	FacilityLocal5
	FacilityLocal6
	FacilityLocal7
)
