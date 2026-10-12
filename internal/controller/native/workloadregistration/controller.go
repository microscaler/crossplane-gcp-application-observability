// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package workloadregistration reconciles WorkloadRegistrations: it registers
// the App Hub discovered workloads a selector matches to an application.
package workloadregistration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/microscaler/crossplane-gcp-application-observability/apis/cluster/apphub/v1alpha1"
	"github.com/microscaler/crossplane-gcp-application-observability/internal/clients"
	"github.com/microscaler/crossplane-gcp-application-observability/internal/features"
	"github.com/microscaler/crossplane-gcp-application-observability/internal/gcprest"
)

const (
	apphubAPI = "https://apphub.googleapis.com/v1"
	crmAPI    = "https://cloudresourcemanager.googleapis.com/v3"

	errNotWR = "managed resource is not a WorkloadRegistration"
)

var (
	gkeURI = regexp.MustCompile(`/clusters/([^/]+)/k8s/namespaces/([^/]+)/apps/([^/]+)/([^/]+)$`)
	kinds  = map[string]string{"deployments": "Deployment", "statefulsets": "StatefulSet", "daemonsets": "DaemonSet", "cronjobs": "CronJob"}
	badID  = regexp.MustCompile(`[^a-z0-9-]`)
)

// Setup adds a controller that reconciles WorkloadRegistrations.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	name := managed.ControllerName(v1alpha1.WorkloadRegistration_GroupVersionKind.String())
	opts := []managed.ReconcilerOption{
		managed.WithExternalConnector(&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithTimeout(3 * time.Minute),
		managed.WithPollInterval(o.PollInterval),
		// The external name is set on first registration, not defaulted from
		// metadata.name: its absence is how a new registration is recognised.
		managed.WithInitializers(),
	}
	if o.Features.Enabled(features.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}
	r := managed.NewReconciler(mgr, xpresource.ManagedKind(v1alpha1.WorkloadRegistration_GroupVersionKind), opts...)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(xpresource.DesiredStateChanged()).
		For(&v1alpha1.WorkloadRegistration{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

func (c *connector) Connect(ctx context.Context, mg xpresource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.WorkloadRegistration)
	if !ok {
		return nil, errors.New(errNotWR)
	}
	hc, project, err := clients.GoogleHTTPClient(ctx, c.kube, mg)
	if err != nil {
		return nil, err
	}
	if p := cr.Spec.ForProvider.Project; p != nil && *p != "" {
		project = *p
	}
	return &external{api: gcprest.New(hc), project: project}, nil
}

type external struct {
	api     *gcprest.Client
	project string
}

// workload is an App Hub registered workload.
type workload struct {
	Name               string          `json:"name"`
	DiscoveredWorkload string          `json:"discoveredWorkload"`
	DisplayName        string          `json:"displayName,omitempty"`
	Description        string          `json:"description,omitempty"`
	State              string          `json:"state,omitempty"`
	Attributes         json.RawMessage `json:"attributes,omitempty"`
}

func (w workload) id() string { return w.Name[strings.LastIndex(w.Name, "/")+1:] }

type selected struct {
	discovered, ns, name, kind string
}

type plan struct {
	selected   []selected
	register   []selected // selected, not yet registered anywhere in the application
	owned      []workload // registered by this resource (or adopted)
	adopt      []workload // owned via an adopt description, still to be re-marked
	drifted    []workload // owned, attributes differ from the spec
	remove     []workload // owned, but detached or no longer selected
	conflicts  []string
	attributes map[string]any
	marker     string
}

func marker(cr *v1alpha1.WorkloadRegistration) string {
	return fmt.Sprintf("Registered by Crossplane WorkloadRegistration %s.", cr.GetName())
}

func (e *external) appURL(cr *v1alpha1.WorkloadRegistration) string {
	p := cr.Spec.ForProvider
	return fmt.Sprintf("%s/projects/%s/locations/%s/applications/%s", apphubAPI, e.project, p.Location, p.ApplicationID)
}

func (e *external) plan(ctx context.Context, cr *v1alpha1.WorkloadRegistration) (*plan, error) { //nolint:gocyclo // one pass over two lists
	p := cr.Spec.ForProvider
	pl := &plan{marker: marker(cr), attributes: attributes(p.Attributes)}

	if err := e.api.Do(ctx, http.MethodGet, e.appURL(cr), nil, nil, nil); err != nil {
		return nil, errors.Wrapf(err, "cannot get application %s", p.ApplicationID)
	}

	wantKinds := p.Kinds
	if len(wantKinds) == 0 {
		wantKinds = []string{"Deployment", "StatefulSet"}
	}
	var clusterProjects []string
	if p.ClusterProject != nil && *p.ClusterProject != "" {
		var proj struct {
			Name string `json:"name"`
		}
		if err := e.api.Do(ctx, http.MethodGet, crmAPI+"/projects/"+*p.ClusterProject, nil, nil, &proj); err != nil {
			return nil, errors.Wrapf(err, "cannot look up project %s", *p.ClusterProject)
		}
		clusterProjects = []string{*p.ClusterProject, strings.TrimPrefix(proj.Name, "projects/")}
	}

	discoveredURL := fmt.Sprintf("%s/projects/%s/locations/%s/discoveredWorkloads", apphubAPI, e.project, p.Location)
	err := e.api.List(ctx, discoveredURL, url.Values{"pageSize": {"500"}}, "discoveredWorkloads", func(raw json.RawMessage) error {
		var dw struct {
			Name              string `json:"name"`
			WorkloadReference struct {
				URI string `json:"uri"`
			} `json:"workloadReference"`
			WorkloadProperties struct {
				GCPProject string `json:"gcpProject"`
			} `json:"workloadProperties"`
		}
		if err := json.Unmarshal(raw, &dw); err != nil {
			return err
		}
		m := gkeURI.FindStringSubmatch(dw.WorkloadReference.URI)
		if m == nil {
			return nil
		}
		cluster, ns, kind, name := m[1], m[2], kinds[m[3]], m[4]
		project := dw.WorkloadProperties.GCPProject[strings.LastIndex(dw.WorkloadProperties.GCPProject, "/")+1:]
		if cluster != p.Cluster || !slices.Contains(p.Namespaces, ns) || !slices.Contains(wantKinds, kind) ||
			slices.Contains(p.Exclude, name) || (clusterProjects != nil && !slices.Contains(clusterProjects, project)) {
			return nil
		}
		pl.selected = append(pl.selected, selected{discovered: dw.Name, ns: ns, name: name, kind: kind})
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "cannot list discovered workloads")
	}

	byDiscovered := map[string]workload{}
	err = e.api.List(ctx, e.appURL(cr)+"/workloads", nil, "workloads", func(raw json.RawMessage) error {
		var w workload
		if err := json.Unmarshal(raw, &w); err != nil {
			return err
		}
		byDiscovered[w.DiscoveredWorkload] = w
		switch {
		case w.Description == pl.marker:
		case slices.Contains(p.AdoptDescriptions, w.Description):
			pl.adopt = append(pl.adopt, w)
		default:
			return nil
		}
		pl.owned = append(pl.owned, w)
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "cannot list application workloads")
	}

	isSelected := map[string]bool{}
	for _, s := range pl.selected {
		isSelected[s.discovered] = true
		w, ok := byDiscovered[s.discovered]
		switch {
		case !ok:
			pl.register = append(pl.register, s)
		case w.Description != pl.marker && !slices.Contains(p.AdoptDescriptions, w.Description):
			pl.conflicts = append(pl.conflicts, fmt.Sprintf("%s/%s registered as %s", s.ns, s.name, w.id()))
		}
	}
	for _, w := range pl.owned {
		switch {
		case w.State == "DETACHED" || !isSelected[w.DiscoveredWorkload]:
			pl.remove = append(pl.remove, w)
		case pl.attributes != nil && !sameAttributes(w.Attributes, pl.attributes):
			pl.drifted = append(pl.drifted, w)
		}
	}
	// Registrations owned elsewhere in the application that a selected workload
	// already uses are reported, not fought over.
	return pl, nil
}

func (pl *plan) upToDate() bool {
	return len(pl.register) == 0 && len(pl.adopt) == 0 && len(pl.drifted) == 0 && len(pl.remove) == 0
}

func (e *external) Observe(ctx context.Context, mg xpresource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.WorkloadRegistration)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotWR)
	}
	pl, err := e.plan(ctx, cr)
	if err != nil {
		if meta.WasDeleted(cr) && gcprest.IsNotFound(errors.Cause(err)) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, err
	}
	if meta.WasDeleted(cr) {
		return managed.ExternalObservation{ResourceExists: len(pl.owned) > 0}, nil
	}
	if meta.GetExternalName(cr) == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	ids := make([]string, 0, len(pl.owned))
	for _, w := range pl.owned {
		ids = append(ids, w.id())
	}
	sort.Strings(ids)
	sort.Strings(pl.conflicts)
	cr.Status.AtProvider = v1alpha1.WorkloadRegistrationObservation{
		Selected:   len(pl.selected),
		Registered: ids,
		Conflicts:  pl.conflicts,
	}
	cr.SetConditions(xpv2.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: pl.upToDate()}, nil
}

func (e *external) Create(ctx context.Context, mg xpresource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.WorkloadRegistration)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotWR)
	}
	cr.SetConditions(xpv2.Creating())
	if err := e.apply(ctx, cr); err != nil {
		return managed.ExternalCreation{}, err
	}
	meta.SetExternalName(cr, cr.GetName())
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg xpresource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.WorkloadRegistration)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotWR)
	}
	return managed.ExternalUpdate{}, e.apply(ctx, cr)
}

func (e *external) apply(ctx context.Context, cr *v1alpha1.WorkloadRegistration) error {
	pl, err := e.plan(ctx, cr)
	if err != nil {
		return err
	}
	var errs []string
	for _, s := range pl.register {
		body := map[string]any{
			"discoveredWorkload": s.discovered,
			"displayName":        s.ns + "/" + s.name,
			"description":        pl.marker,
		}
		if pl.attributes != nil {
			body["attributes"] = pl.attributes
		}
		q := url.Values{"workloadId": {workloadID(s.ns, s.name)}}
		if err := e.api.Do(ctx, http.MethodPost, e.appURL(cr)+"/workloads", q, body, nil); err != nil && !gcprest.IsConflict(err) {
			errs = append(errs, fmt.Sprintf("register %s/%s: %v", s.ns, s.name, err))
		}
	}
	patch := map[string]workload{}
	for _, w := range pl.adopt {
		patch[w.Name] = w
	}
	for _, w := range pl.drifted {
		patch[w.Name] = w
	}
	for name := range patch {
		body := map[string]any{"description": pl.marker}
		mask := "description"
		if pl.attributes != nil {
			body["attributes"] = pl.attributes
			mask += ",attributes"
		}
		if err := e.api.Do(ctx, http.MethodPatch, apphubAPI+"/"+name, url.Values{"updateMask": {mask}}, body, nil); err != nil {
			errs = append(errs, fmt.Sprintf("update %s: %v", name, err))
		}
	}
	for _, w := range pl.remove {
		if err := e.api.Do(ctx, http.MethodDelete, apphubAPI+"/"+w.Name, nil, nil, nil); err != nil && !gcprest.IsNotFound(err) {
			errs = append(errs, fmt.Sprintf("remove %s: %v", w.id(), err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (e *external) Delete(ctx context.Context, mg xpresource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.WorkloadRegistration)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotWR)
	}
	cr.SetConditions(xpv2.Deleting())
	pl, err := e.plan(ctx, cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	for _, w := range pl.owned {
		if err := e.api.Do(ctx, http.MethodDelete, apphubAPI+"/"+w.Name, nil, nil, nil); err != nil && !gcprest.IsNotFound(err) {
			return managed.ExternalDelete{}, errors.Wrapf(err, "cannot remove workload %s", w.id())
		}
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error { return nil }

// workloadID derives a stable App Hub workload ID from namespace and name.
func workloadID(ns, name string) string {
	id := strings.TrimRight(truncate(badID.ReplaceAllString(strings.ToLower(ns+"-"+name), "-"), 63), "-")
	if id == "" || id[0] < 'a' || id[0] > 'z' {
		id = truncate("w-"+id, 63)
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func owners(in []v1alpha1.Owner) []any {
	out := make([]any, 0, len(in))
	for _, o := range in {
		dn := o.Email
		if i := strings.Index(dn, "@"); i > 0 {
			dn = dn[:i]
		}
		if o.DisplayName != nil {
			dn = *o.DisplayName
		}
		out = append(out, map[string]any{"email": o.Email, "displayName": dn})
	}
	return out
}

// attributes renders the API form of the spec attributes, or nil when unset.
func attributes(a *v1alpha1.WorkloadAttributes) map[string]any {
	if a == nil {
		return nil
	}
	out := map[string]any{}
	if a.Environment != nil {
		out["environment"] = map[string]any{"type": *a.Environment}
	}
	if a.Criticality != nil {
		out["criticality"] = map[string]any{"type": *a.Criticality}
	}
	for k, v := range map[string][]v1alpha1.Owner{
		"businessOwners": a.BusinessOwners, "developerOwners": a.DeveloperOwners, "operatorOwners": a.OperatorOwners,
	} {
		if len(v) > 0 {
			out[k] = owners(v)
		}
	}
	return out
}

// sameAttributes compares API attributes with the desired ones, ignoring owner
// order and anything the desired attributes do not set.
func sameAttributes(have json.RawMessage, want map[string]any) bool {
	var h map[string]any
	if len(have) > 0 {
		if err := json.Unmarshal(have, &h); err != nil {
			return false
		}
	}
	b, _ := json.Marshal(want)
	var w map[string]any
	_ = json.Unmarshal(b, &w)
	for k, v := range w {
		if !reflect.DeepEqual(normalise(h[k]), normalise(v)) {
			return false
		}
	}
	return true
}

func normalise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range t {
			if x != nil && x != "" {
				out[k] = normalise(x)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			b, _ := json.Marshal(normalise(x))
			out = append(out, string(b))
		}
		sort.Strings(out)
		return out
	default:
		return v
	}
}
