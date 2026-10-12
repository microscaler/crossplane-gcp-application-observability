# crossplane-gcp-application-observability

A Crossplane provider for the Google Cloud resources behind **Application
Monitoring**: App Hub, the Cloud Monitoring metrics scope and the
Observability log, trace and default scopes. None of these are covered by the
Upbound GCP provider family.

Most resources are generated with [upjet](https://github.com/crossplane/upjet)
from the Terraform `hashicorp/google` provider. Two are hand-written controllers,
for things the Terraform provider cannot express.

## Resources

| Group (`*.gcpobservability.microscaler.io`) | Kind | Source |
|---|---|---|
| `apphub` | `ServiceProjectAttachment` | `google_apphub_service_project_attachment` |
| `apphub` | `Application` | `google_apphub_application` |
| `apphub` | `Workload` | `google_apphub_workload` |
| `apphub` | `Service` | `google_apphub_service` |
| `apphub` | `WorkloadRegistration` | hand-written |
| `monitoring` | `MonitoredProject` | `google_monitoring_monitored_project` |
| `logging` | `LogScope` | `google_logging_log_scope` |
| `observability` | `TraceScope` | `google_observability_trace_scope` |
| `observability` | `Scope` | hand-written |

The upjet resources exist cluster-scoped and namespaced (`*.m.gcpobservability.microscaler.io`);
the hand-written ones are cluster-scoped.

### WorkloadRegistration

App Hub gives each discovered workload an opaque ID, so a `Workload` cannot be
written down ahead of time. A `WorkloadRegistration` selects discovered GKE
workloads by cluster, namespaces and kinds, registers each one to an
application (workload ID `<namespace>-<name>`), applies attributes, and removes
its registrations when a workload goes away or the resource is deleted.
`adoptDescriptions` takes over registrations made by earlier tooling.

```yaml
apiVersion: apphub.gcpobservability.microscaler.io/v1alpha1
kind: WorkloadRegistration
metadata:
  name: shop-workloads
spec:
  forProvider:
    location: us-east4
    applicationId: shop
    cluster: runtime
    namespaces: [shop]
    kinds: [Deployment, StatefulSet]
    attributes:
      environment: PRODUCTION
      criticality: HIGH
      operatorOwners: [{ email: oncall@example.com }]
```

### Scope

Every project has one Observability scope, `_Default`, naming the log and trace
scopes the console uses. `Scope` sets them; deleting it restores the project's
own `_Default` log scope and clears the trace scope.

```yaml
apiVersion: observability.gcpobservability.microscaler.io/v1alpha1
kind: Scope
metadata:
  name: host-default
spec:
  forProvider:
    logScope: //logging.googleapis.com/projects/host-project/locations/global/logScopes/apps
    traceScope: projects/host-project/locations/global/traceScopes/apps
```

## ProviderConfig

```yaml
apiVersion: gcpobservability.microscaler.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
spec:
  projectID: host-project
  credentials:
    source: InjectedIdentity   # GKE Workload Identity; or Secret with a service account key
```

With `InjectedIdentity`, bind the provider's Kubernetes service account to a
Google service account through a `DeploymentRuntimeConfig`, as for the Upbound
GCP providers.

## Developing

```console
make submodules
make generate   # regenerate CRDs, types and controllers
go build ./...
```

Add a Terraform resource in `config/external_name.go` (and `config/apphub` for
references or kind names), then `make generate`.
