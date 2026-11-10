/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kube

import (
	"context"
	"fmt"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// whoAmITimeout bounds the one call this package makes before the dashboard is
// up. An identity that cannot be read in this long is reported as unavailable
// rather than holding the start of the dashboard hostage.
const whoAmITimeout = 5 * time.Second

// Identity is who the API server says the kubeconfig authenticates as.
//
// It is the answer to "what can this dashboard do": exactly what this identity
// can. Showing it is how a person with four contexts notices they are about to
// look at a cluster as the wrong user.
//
// Extra attributes are deliberately not carried. They can hold provider claims
// that were never meant to be rendered on a page.
type Identity struct {
	Username string   `json:"username"`
	UID      string   `json:"uid,omitempty"`
	Groups   []string `json:"groups,omitempty"`
}

// WhoAmI asks the API server who the caller is with a SelfSubjectReview.
//
// SelfSubjectReview is GA from Kubernetes 1.28 and needs no RBAC beyond the
// default system:basic-user binding that every authenticated user has. An older
// cluster or a locked-down one returns an error, which the caller reports as an
// unavailable identity instead of failing to start: nothing else in the
// dashboard depends on it.
func WhoAmI(ctx context.Context, client kubernetes.Interface) (Identity, error) {
	ctx, cancel := context.WithTimeout(ctx, whoAmITimeout)
	defer cancel()

	review, err := client.AuthenticationV1().SelfSubjectReviews().
		Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return Identity{}, fmt.Errorf("asking the API server who you are (SelfSubjectReview needs Kubernetes 1.28 or newer): %w", err)
	}

	user := review.Status.UserInfo
	if user.Username == "" && user.UID == "" {
		return Identity{}, fmt.Errorf("the API server answered the SelfSubjectReview with no identity")
	}
	return Identity{
		Username: user.Username,
		UID:      user.UID,
		Groups:   append([]string(nil), user.Groups...),
	}, nil
}
