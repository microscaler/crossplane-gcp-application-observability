package config

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// project resolves the GCP project of a resource: spec.forProvider.project when
// set, otherwise the projectID of the ProviderConfig (passed to Terraform as the
// provider-level "project").
const project = `{{ if .parameters.project }}{{ .parameters.project }}{{ else }}{{ .setup.configuration.project }}{{ end }}`

// ExternalNameConfigs contains all external name configurations for this
// provider. Every resource is keyed by its user-chosen identifier, so existing
// GCP objects can be adopted by setting crossplane.io/external-name.
var ExternalNameConfigs = map[string]config.ExternalName{
	// App Hub
	"google_apphub_service_project_attachment": config.TemplatedStringAsIdentifier("service_project_attachment_id",
		"projects/"+project+"/locations/global/serviceProjectAttachments/{{ .external_name }}"),
	"google_apphub_application": config.TemplatedStringAsIdentifier("application_id",
		"projects/"+project+"/locations/{{ .parameters.location }}/applications/{{ .external_name }}"),
	"google_apphub_workload": config.TemplatedStringAsIdentifier("workload_id",
		"projects/"+project+"/locations/{{ .parameters.location }}/applications/{{ .parameters.application_id }}/workloads/{{ .external_name }}"),
	"google_apphub_service": config.TemplatedStringAsIdentifier("service_id",
		"projects/"+project+"/locations/{{ .parameters.location }}/applications/{{ .parameters.application_id }}/services/{{ .external_name }}"),

	// Cloud Monitoring metrics scope membership
	"google_monitoring_monitored_project": config.TemplatedStringAsIdentifier("name",
		"locations/global/metricsScopes/{{ .parameters.metrics_scope }}/projects/{{ .external_name }}"),

	// Observability scopes
	"google_logging_log_scope": config.TemplatedStringAsIdentifier("name",
		"{{ .parameters.parent }}/locations/{{ if .parameters.location }}{{ .parameters.location }}{{ else }}global{{ end }}/logScopes/{{ .external_name }}"),
	"google_observability_trace_scope": config.TemplatedStringAsIdentifier("trace_scope_id",
		"projects/"+project+"/locations/{{ .parameters.location }}/traceScopes/{{ .external_name }}"),
}

// ExternalNameConfigurations applies all external name configs listed in the
// table ExternalNameConfigs.
func ExternalNameConfigurations() config.ResourceOption {
	return func(r *config.Resource) {
		if e, ok := ExternalNameConfigs[r.Name]; ok {
			r.ExternalName = e
		}
	}
}

// ExternalNameConfigured returns the list of all resources whose external name
// is configured manually.
func ExternalNameConfigured() []string {
	l := make([]string, 0, len(ExternalNameConfigs))
	for name := range ExternalNameConfigs {
		// $ is added to match the exact string since the format is regex.
		l = append(l, name+"$")
	}
	return l
}
