// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package filtered

import (
	"regexp"
)

type Filter struct {
	Exclude []*regexp.Regexp `yaml:"Exclude,omitempty" json:",omitempty"`
	Include []*regexp.Regexp `yaml:"Include,omitempty" json:",omitempty"`
}

func (f *Filter) Enabled() bool {
	return f != nil && (len(f.Include) > 0 || len(f.Exclude) > 0)
}

func (f *Filter) Passed(val string) bool {
	if len(f.Include) > 0 {
		for _, v := range f.Include {
			if v.MatchString(val) {
				return true
			}
		}

		return false
	} else {
		for _, v := range f.Exclude {
			if v.MatchString(val) {
				return false
			}
		}

		return true
	}
}
