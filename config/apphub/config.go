// Package apphub configures the App Hub, metrics scope and observability scope
// resources. The same configuration is applied to the cluster-scoped and the
// namespaced API.
package apphub

import (
	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
)

// Configure adds the App Hub resource configurations.
func Configure(p *ujconfig.Provider) {
	for _, name := range []string{"google_apphub_workload", "google_apphub_service"} {
		p.AddResourceConfigurator(name, func(r *ujconfig.Resource) {
			r.ShortGroup = "apphub"
			r.References["application_id"] = ujconfig.Reference{
				TerraformName: "google_apphub_application",
			}
		})
	}
	p.AddResourceConfigurator("google_apphub_application", func(r *ujconfig.Resource) {
		r.ShortGroup = "apphub"
		r.Kind = "Application"
	})
	p.AddResourceConfigurator("google_apphub_service_project_attachment", func(r *ujconfig.Resource) {
		r.ShortGroup = "apphub"
		r.Kind = "ServiceProjectAttachment"
	})
	p.AddResourceConfigurator("google_monitoring_monitored_project", func(r *ujconfig.Resource) {
		r.ShortGroup = "monitoring"
		r.Kind = "MonitoredProject"
	})
	p.AddResourceConfigurator("google_logging_log_scope", func(r *ujconfig.Resource) {
		r.ShortGroup = "logging"
		r.Kind = "LogScope"
	})
	p.AddResourceConfigurator("google_observability_trace_scope", func(r *ujconfig.Resource) {
		r.ShortGroup = "observability"
		r.Kind = "TraceScope"
	})
}
