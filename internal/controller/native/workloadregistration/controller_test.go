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

// fakeAppHub serves one application. Like App Hub, the discovery listing holds only workloads that are not
// registered to any application.
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

func ref(cluster, ns, kind, name string) (map[string]any, map[string]any) {
	return map[string]any{"uri": "//container.googleapis.com/projects/123/locations/us-east4/clusters/" + cluster + "/k8s/namespaces/" + ns + "/apps/" + kind + "/" + name},
		map[string]any{"gcpProject": "projects/svc"}
}

func discovered(id, ns, name string) map[string]any {
	r, props := ref("c1", ns, "deployments", name)
	return map[string]any{
		"name":               "projects/host/locations/us-east4/discoveredWorkloads/" + id,
		"workloadReference":  r,
		"workloadProperties": props,
	}
}

func registered(ns, name, description, state string) map[string]any {
	r, props := ref("c1", ns, "deployments", name)
	return map[string]any{
		"name":               "projects/host/locations/us-east4/applications/app/workloads/" + ns + "-" + name,
		"discoveredWorkload": "projects/999/locations/us-east4/discoveredWorkloads/dw-" + name,
		"description":        description,
		"state":              state,
		"workloadReference":  r,
		"workloadProperties": props,
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

func planFor(t *testing.T, disc, reg []map[string]any) *plan {
	t.Helper()
	srv := fakeAppHub(t, disc, reg)
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

// The regression behind v0.1.2: registered workloads are absent from discovery, and must still count as selected.
func TestRegisteredWorkloadsKeptThoughNotDiscovered(t *testing.T) {
	m := marker(registration())
	pl := planFor(t, nil, []map[string]any{
		registered("ns1", "web", m, "ACTIVE"),
		registered("ns1", "api", "legacy", "ACTIVE"),
	})
	if len(pl.remove) != 0 || len(pl.register) != 0 {
		t.Fatalf("want nothing removed or registered; got remove=%v register=%v", pl.remove, pl.register)
	}
	if pl.selected != 2 || len(pl.owned) != 2 || len(pl.adopt) != 1 {
		t.Fatalf("want 2 selected and owned, 1 adopted; got selected=%d owned=%d adopt=%d", pl.selected, len(pl.owned), len(pl.adopt))
	}
}

func TestDiscoveredSelectedWorkloadsAreRegistered(t *testing.T) {
	pl := planFor(t,
		[]map[string]any{discovered("dw-new", "ns1", "new"), discovered("dw-other", "other", "skip")},
		[]map[string]any{registered("ns1", "web", marker(registration()), "ACTIVE")})
	if len(pl.register) != 1 || pl.register[0].name != "new" || pl.register[0].discovered == "" {
		t.Fatalf("want ns1/new registered; got %+v", pl.register)
	}
	if pl.selected != 2 || len(pl.remove) != 0 {
		t.Fatalf("want 2 selected, none removed; got selected=%d remove=%v", pl.selected, pl.remove)
	}
}

func TestOwnedWorkloadsRemovedWhenDetachedOrDeselected(t *testing.T) {
	m := marker(registration())
	pl := planFor(t, nil, []map[string]any{
		registered("ns1", "gone", m, "DETACHED"),
		registered("ns2", "moved", m, "ACTIVE"), // ns2 is not in the selector
		registered("ns1", "kept", m, "ACTIVE"),
	})
	if len(pl.remove) != 2 {
		t.Fatalf("want the detached and the deselected workload removed; got %v", pl.remove)
	}
	if pl.selected != 1 {
		t.Fatalf("want 1 selected; got %d", pl.selected)
	}
}

func TestForeignRegistrationsAreReportedNotTouched(t *testing.T) {
	pl := planFor(t, nil, []map[string]any{
		registered("ns1", "web", "someone else", "ACTIVE"),
		registered("ns9", "unrelated", "someone else", "DETACHED"),
	})
	if len(pl.remove) != 0 || len(pl.owned) != 0 {
		t.Fatalf("want foreign registrations left alone; got remove=%v owned=%v", pl.remove, pl.owned)
	}
	if len(pl.conflicts) != 1 {
		t.Fatalf("want one conflict; got %v", pl.conflicts)
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
