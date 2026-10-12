// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clients

import (
	"context"
	"net/http"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// GoogleHTTPClient returns an authenticated HTTP client for Google APIs and the
// default project, both taken from the managed resource's ProviderConfig. It
// serves the hand-written controllers; the Terraform-backed ones authenticate
// through TerraformSetupBuilder.
func GoogleHTTPClient(ctx context.Context, kube client.Client, mg resource.Managed) (*http.Client, string, error) {
	pcSpec, err := resolveProviderConfig(ctx, kube, mg)
	if err != nil {
		return nil, "", errors.Wrap(err, "cannot resolve provider config")
	}
	if pcSpec.Credentials.Source == "InjectedIdentity" {
		hc, err := google.DefaultClient(ctx, cloudPlatformScope)
		return hc, pcSpec.ProjectID, errors.Wrap(err, "cannot get application default credentials")
	}
	data, err := resource.CommonCredentialExtractor(ctx, pcSpec.Credentials.Source, kube, pcSpec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return nil, "", errors.Wrap(err, errExtractCredentials)
	}
	creds, err := google.CredentialsFromJSON(ctx, data, cloudPlatformScope) //nolint:staticcheck // key JSON comes from the user's own secret
	if err != nil {
		return nil, "", errors.Wrap(err, "cannot parse service account key")
	}
	return oauth2.NewClient(ctx, creds.TokenSource), pcSpec.ProjectID, nil
}
