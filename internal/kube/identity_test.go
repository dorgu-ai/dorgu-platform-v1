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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func clientAnswering(review *authenticationv1.SelfSubjectReview, err error) *fake.Clientset {
	client := fake.NewClientset()
	client.PrependReactor("create", "selfsubjectreviews",
		func(clienttesting.Action) (bool, runtime.Object, error) { return true, review, err })
	return client
}

func TestWhoAmIReportsUsernameUIDAndGroups(t *testing.T) {
	client := clientAnswering(&authenticationv1.SelfSubjectReview{
		Status: authenticationv1.SelfSubjectReviewStatus{UserInfo: authenticationv1.UserInfo{
			Username: "arn:aws:iam::123:role/dev",
			UID:      "aws-iam-authenticator:123:ABC",
			Groups:   []string{"system:authenticated", "dorgu:admins"},
			Extra:    map[string]authenticationv1.ExtraValue{"secret": {"claim"}},
		}},
	}, nil)

	id, err := WhoAmI(context.Background(), client)

	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::123:role/dev", id.Username)
	assert.Equal(t, "aws-iam-authenticator:123:ABC", id.UID)
	assert.Equal(t, []string{"system:authenticated", "dorgu:admins"}, id.Groups)
}

func TestWhoAmINamesTheVersionRequirementWhenTheAPIServerRefuses(t *testing.T) {
	client := clientAnswering(nil, errors.New("the server could not find the requested resource"))

	_, err := WhoAmI(context.Background(), client)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "1.28")
	assert.Contains(t, err.Error(), "could not find")
}

func TestWhoAmIRejectsAnEmptyAnswer(t *testing.T) {
	client := clientAnswering(&authenticationv1.SelfSubjectReview{}, nil)

	_, err := WhoAmI(context.Background(), client)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no identity")
}

func TestWhoAmIDoesNotAliasTheResponseGroups(t *testing.T) {
	review := &authenticationv1.SelfSubjectReview{
		Status: authenticationv1.SelfSubjectReviewStatus{UserInfo: authenticationv1.UserInfo{
			Username: "u", Groups: []string{"a"},
		}},
	}
	id, err := WhoAmI(context.Background(), clientAnswering(review, nil))
	require.NoError(t, err)
	id.Groups[0] = "mutated"
	assert.Equal(t, "a", review.Status.UserInfo.Groups[0])
}
