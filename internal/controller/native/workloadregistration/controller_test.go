// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package workloadregistration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/microscaler/crossplane-gcp-application-observability/apis/cluster/apphub/v1alpha1"
	"github.com/microscaler/crossplane-gcp-application-observability/internal/gcprest"
)

// fakeAppHub serves one application with the given discovered and registered workloads.
func fakeAppHub(t *testing.T, discovered, registered []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any = map[string]any{}
		switch {
		case strings.HasSuffix(r.URL.Path, "/discoveredWorkloads"):
			body = map[string]any{"discoveredWorkloads": discovered}
		case strings.HasSuffix(r.URL.Path, "/applications/app/workloads"):
			body = map[string]any{"workloads": registered}
		case strings.HasSuffix(r.URL.Path, "/applications/app"):
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func dw(id, ns, name string) map[string]any {
	return map[string]any{
		// Listed with the project ID...
		"name":               "projects/host/locations/us-east4/discoveredWorkloads/" + id,
		"workloadReference":  map[string]any{"uri": "//container.googleapis.com/projects/svc/locations/us-east4/clusters/c1/k8s/namespaces/" + ns + "/apps/deployments/" + name},
		"workloadProperties": map[string]any{"gcpProject": "projects/123"},
	}
}

func registeredWorkload(id, wid, description string) map[string]any {
	return map[string]any{
		"name": "projects/host/locations/us-east4/applications/app/workloads/" + wid,
		// ...but referenced with the project number once registered.
		"discoveredWorkload": "projects/999/locations/us-east4/discoveredWorkloads/" + id,
		"description":        description,
		"state":              "ACTIVE",
	}
}

func registration() *v1alpha1.WorkloadRegistration {
	return &v1alpha1.WorkloadRegistration{
		ObjectMeta: metav1.ObjectMeta{Name: "reg"},
		Spec: v1alpha1.WorkloadRegistrationSpec{ForProvider: v1alpha1.WorkloadRegistrationParameters{
			Location: "us-east4", ApplicationID: "app", Cluster: "c1", Namespaces: []string{"ns1"},
			AdoptDescriptions: []string{"legacy"},
		}},
	}
}

func planFor(t *testing.T, discovered, registered []map[string]any) *plan {
	t.Helper()
	srv := fakeAppHub(t, discovered, registered)
	t.Cleanup(srv.Close)
	old := apphubAPI
	apphubAPI = srv.URL
	t.Cleanup(func() { apphubAPI = old })
	e := &external{api: gcprest.New(srv.Client()), project: "host"}
	pl, err := e.plan(context.Background(), registration())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return pl
}

// Registered workloads name their discovered workload with the project number, the listing with the project ID:
// they must still match, so nothing is registered twice or removed.
func TestPlanMatchesAcrossProjectIDAndNumber(t *testing.T) {
	cr := registration()
	pl := planFor(t,
		[]map[string]any{dw("dw-1", "ns1", "web"), dw("dw-2", "ns1", "api")},
		[]map[string]any{registeredWorkload("dw-1", "ns1-web", marker(cr)), registeredWorkload("dw-2", "ns1-api", "legacy")})
	if len(pl.register) != 0 || len(pl.remove) != 0 || len(pl.conflicts) != 0 {
		t.Fatalf("want nothing to register, remove or conflict; got register=%v remove=%v conflicts=%v", pl.register, pl.remove, pl.conflicts)
	}
	if len(pl.owned) != 2 || len(pl.adopt) != 1 {
		t.Fatalf("want 2 owned (1 adopted); got owned=%d adopt=%d", len(pl.owned), len(pl.adopt))
	}
}

func TestPlanRegistersNewAndRemovesDeselected(t *testing.T) {
	cr := registration()
	pl := planFor(t,
		[]map[string]any{dw("dw-1", "ns1", "web"), dw("dw-3", "ns1", "new"), dw("dw-4", "other", "skip")},
		[]map[string]any{registeredWorkload("dw-1", "ns1-web", marker(cr)), registeredWorkload("dw-2", "ns1-gone", marker(cr))})
	if len(pl.register) != 1 || pl.register[0].name != "new" {
		t.Fatalf("want ns1/new registered; got %v", pl.register)
	}
	if len(pl.remove) != 1 || pl.remove[0].id() != "ns1-gone" {
		t.Fatalf("want ns1-gone removed; got %v", pl.remove)
	}
}

// A discovery listing that comes back empty must not deregister everything.
func TestPlanEmptyDiscoveryRemovesNothing(t *testing.T) {
	cr := registration()
	pl := planFor(t, nil,
		[]map[string]any{registeredWorkload("dw-1", "ns1-web", marker(cr)), registeredWorkload("dw-2", "ns1-api", marker(cr))})
	if len(pl.remove) != 0 {
		t.Fatalf("want nothing removed on empty discovery; got %v", pl.remove)
	}
}

func TestWorkloadID(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"pw", "trader"}:                   "pw-trader",
		{"postgres-operator", "Op_Erator"}: "postgres-operator-op-erator",
		{"9ns", "x"}:                       "w-9ns-x",
		{"ns", strings.Repeat("a", 80)}:    "ns-" + strings.Repeat("a", 60),
	} {
		if got := workloadID(in[0], in[1]); got != want {
			t.Errorf("workloadID(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
