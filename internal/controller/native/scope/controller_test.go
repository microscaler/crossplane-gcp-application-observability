// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package scope

import "testing"

func TestSameName(t *testing.T) {
	const want = "//logging.googleapis.com/projects/p/locations/global/logScopes/s"
	for _, have := range []string{
		want,
		"logging.googleapis.com/projects/p/locations/global/logScopes/s", // what the API returns
		"projects/p/locations/global/logScopes/s",
	} {
		if !sameName(have, want) {
			t.Errorf("sameName(%q, %q) = false, want true", have, want)
		}
	}
	if sameName("logging.googleapis.com/projects/p/locations/global/logScopes/other", want) {
		t.Error("different scopes compared equal")
	}
	if !sameName("projects/p/locations/global/traceScopes/s", "projects/p/locations/global/traceScopes/s") {
		t.Error("identical trace scopes compared unequal")
	}
}
