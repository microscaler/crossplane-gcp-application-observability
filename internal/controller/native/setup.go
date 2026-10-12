// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package native sets up the hand-written controllers: resources the Google
// APIs offer but the Terraform provider does not.
package native

import (
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/microscaler/crossplane-gcp-application-observability/internal/controller/native/scope"
	"github.com/microscaler/crossplane-gcp-application-observability/internal/controller/native/workloadregistration"
)

// Setup adds the hand-written controllers to the manager.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	for _, setup := range []func(ctrl.Manager, tjcontroller.Options) error{
		workloadregistration.Setup,
		scope.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
