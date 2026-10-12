// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package scope reconciles Scopes: a project's Observability scope settings.
package scope

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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

	"github.com/microscaler/crossplane-gcp-application-observability/apis/cluster/observability/v1alpha1"
	"github.com/microscaler/crossplane-gcp-application-observability/internal/clients"
	"github.com/microscaler/crossplane-gcp-application-observability/internal/features"
	"github.com/microscaler/crossplane-gcp-application-observability/internal/gcprest"
)

const (
	observabilityAPI = "https://observability.googleapis.com/v1"
	defaultScopeID   = "_Default"
	errNotScope      = "managed resource is not a Scope"
)

// Setup adds a controller that reconciles Scopes.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	name := managed.ControllerName(v1alpha1.Scope_GroupVersionKind.String())
	opts := []managed.ReconcilerOption{
		managed.WithExternalConnector(&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))), //nolint:staticcheck // the recorder crossplane-runtime's event package takes
		managed.WithTimeout(2 * time.Minute),
		managed.WithPollInterval(o.PollInterval),
		managed.WithInitializers(),
	}
	if o.Features.Enabled(features.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}
	r := managed.NewReconciler(mgr, xpresource.ManagedKind(v1alpha1.Scope_GroupVersionKind), opts...)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(xpresource.DesiredStateChanged()).
		For(&v1alpha1.Scope{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

func (c *connector) Connect(ctx context.Context, mg xpresource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Scope)
	if !ok {
		return nil, errors.New(errNotScope)
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

type apiScope struct {
	Name       string `json:"name,omitempty"`
	LogScope   string `json:"logScope,omitempty"`
	TraceScope string `json:"traceScope,omitempty"`
}

func (e *external) url(cr *v1alpha1.Scope) string {
	id := meta.GetExternalName(cr)
	if id == "" {
		id = defaultScopeID
	}
	loc := cr.Spec.ForProvider.Location
	if loc == "" {
		loc = "global"
	}
	return fmt.Sprintf("%s/projects/%s/locations/%s/scopes/%s", observabilityAPI, e.project, loc, id)
}

// resetLogScope is the project's own default log scope, restored on delete.
func (e *external) resetLogScope() string {
	return fmt.Sprintf("//logging.googleapis.com/projects/%s/locations/global/logScopes/_Default", e.project)
}

func (e *external) get(ctx context.Context, cr *v1alpha1.Scope) (apiScope, error) {
	var s apiScope
	err := e.api.Do(ctx, http.MethodGet, e.url(cr), nil, nil, &s)
	return s, errors.Wrap(err, "cannot get observability scope")
}

func (e *external) Observe(ctx context.Context, mg xpresource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Scope)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotScope)
	}
	s, err := e.get(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if meta.WasDeleted(cr) {
		reset := sameName(s.LogScope, e.resetLogScope()) && s.TraceScope == ""
		return managed.ExternalObservation{ResourceExists: !reset}, nil
	}
	lateInit := false
	if meta.GetExternalName(cr) == "" {
		meta.SetExternalName(cr, defaultScopeID)
		lateInit = true
	}
	cr.Status.AtProvider = v1alpha1.ScopeObservation{Name: s.Name, LogScope: s.LogScope, TraceScope: s.TraceScope}
	cr.SetConditions(xpv2.Available())
	p := cr.Spec.ForProvider
	upToDate := (p.LogScope == nil || sameName(s.LogScope, *p.LogScope)) &&
		(p.TraceScope == nil || sameName(s.TraceScope, *p.TraceScope))
	// The scope always exists; Create is never needed, only Update.
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate, ResourceLateInitialized: lateInit}, nil
}

func (e *external) patch(ctx context.Context, cr *v1alpha1.Scope, want apiScope, mask []string) error {
	if len(mask) == 0 {
		return nil
	}
	err := e.api.Do(ctx, http.MethodPatch, e.url(cr), url.Values{"updateMask": {strings.Join(mask, ",")}}, want, nil)
	return errors.Wrap(err, "cannot update observability scope")
}

func (e *external) Create(ctx context.Context, mg xpresource.Managed) (managed.ExternalCreation, error) {
	_, err := e.Update(ctx, mg)
	return managed.ExternalCreation{}, err
}

func (e *external) Update(ctx context.Context, mg xpresource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Scope)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotScope)
	}
	var want apiScope
	var mask []string
	if p := cr.Spec.ForProvider.LogScope; p != nil {
		want.LogScope = *p
		mask = append(mask, "logScope")
	}
	if p := cr.Spec.ForProvider.TraceScope; p != nil {
		want.TraceScope = *p
		mask = append(mask, "traceScope")
	}
	return managed.ExternalUpdate{}, e.patch(ctx, cr, want, mask)
}

func (e *external) Delete(ctx context.Context, mg xpresource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Scope)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotScope)
	}
	cr.SetConditions(xpv2.Deleting())
	return managed.ExternalDelete{}, e.patch(ctx, cr, apiScope{LogScope: e.resetLogScope()}, []string{"logScope", "traceScope"})
}

func (e *external) Disconnect(_ context.Context) error { return nil }

// sameName compares resource names, ignoring a leading service prefix
// (//logging.googleapis.com/) the API may add or drop.
func sameName(a, b string) bool {
	trim := func(s string) string {
		if strings.HasPrefix(s, "//") {
			if i := strings.Index(s[2:], "/"); i >= 0 {
				return s[2+i+1:]
			}
		}
		return s
	}
	return trim(a) == trim(b)
}
