// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/upjet/v2/pkg/controller"

	application "github.com/microscaler/crossplane-gcp-application-observability/internal/controller/namespaced/apphub/application"
	service "github.com/microscaler/crossplane-gcp-application-observability/internal/controller/namespaced/apphub/service"
	serviceprojectattachment "github.com/microscaler/crossplane-gcp-application-observability/internal/controller/namespaced/apphub/serviceprojectattachment"
	workload "github.com/microscaler/crossplane-gcp-application-observability/internal/controller/namespaced/apphub/workload"
	logscope "github.com/microscaler/crossplane-gcp-application-observability/internal/controller/namespaced/logging/logscope"
	monitoredproject "github.com/microscaler/crossplane-gcp-application-observability/internal/controller/namespaced/monitoring/monitoredproject"
	tracescope "github.com/microscaler/crossplane-gcp-application-observability/internal/controller/namespaced/observability/tracescope"
	providerconfig "github.com/microscaler/crossplane-gcp-application-observability/internal/controller/namespaced/providerconfig"
)

// Setup creates all controllers with the supplied logger and adds them to
// the supplied manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		application.Setup,
		service.Setup,
		serviceprojectattachment.Setup,
		workload.Setup,
		logscope.Setup,
		monitoredproject.Setup,
		tracescope.Setup,
		providerconfig.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupGated creates all controllers with the supplied logger and adds them to
// the supplied manager gated.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		application.SetupGated,
		service.SetupGated,
		serviceprojectattachment.SetupGated,
		workload.SetupGated,
		logscope.SetupGated,
		monitoredproject.SetupGated,
		tracescope.SetupGated,
		providerconfig.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupWebhookWithManager registers conversion webhooks for all resource kinds in the group.
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	for _, setup := range []func(ctrl.Manager) error{
		application.SetupWebhookWithManager,
		service.SetupWebhookWithManager,
		serviceprojectattachment.SetupWebhookWithManager,
		workload.SetupWebhookWithManager,
		logscope.SetupWebhookWithManager,
		monitoredproject.SetupWebhookWithManager,
		tracescope.SetupWebhookWithManager,
		providerconfig.SetupWebhookWithManager,
	} {
		if err := setup(mgr); err != nil {
			return err
		}
	}
	return nil
}
