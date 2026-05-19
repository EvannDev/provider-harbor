/*
Copyright 2026 The Crossplane Authors.

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

package group

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
	"github.com/EvannDev/provider-harbor/internal/clients/iam"
)

const (
	// searchPageSize is the number of results per page when searching for groups.
	searchPageSize int64 = 100
)

// Observe checks whether the external Group resource exists and is up to date.
func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	groupResource, ok := mg.(*v1alpha1.Group)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotGroup)
	}

	groupID, err := getGroupID(groupResource)
	if err != nil { //nolint:nilerr // non-numeric external name means resource is not yet created; trigger Create
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// Fetch the group using get API.
	getResponse, err := c.client.GetUserGroup(ctx, iam.GenerateUserGroupGetOpts(groupID))
	if err != nil {
		if iam.IsGroupNotFound(err) {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}

		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	groupResource.Status.AtProvider = iam.GenerateGroupObservation(getResponse.GetPayload())

	former := groupResource.Spec.ForProvider.DeepCopy()
	iam.LateInitializeGroup(&groupResource.Spec.ForProvider, &groupResource.Status.AtProvider)

	groupResource.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        iam.IsGroupUpToDate(&groupResource.Spec.ForProvider, &groupResource.Status.AtProvider),
		ResourceLateInitialized: iam.IsGroupLateInitialized(former, &groupResource.Spec.ForProvider),
		ConnectionDetails:       managed.ConnectionDetails{},
	}, nil
}

// searchGroupByName searches for a group by name across all pages and returns
// the group ID and whether the group was found.
func (c *external) searchGroupByName(ctx context.Context, spec v1alpha1.GroupParameters) (groupID int64, found bool, err error) {
	page := int64(1)
	pageSize := searchPageSize

	for {
		paginationArgs := &common.PaginationArgs{
			Page:     &page,
			PageSize: &pageSize,
		}

		searchResponse, err := c.client.SearchUserGroups(ctx, iam.GenerateUserGroupSearchOpts(spec, paginationArgs))
		if err != nil {
			return 0, false, err
		}

		payload := searchResponse.GetPayload()

		groupExists, groupId := iam.IsGroupInSearchResults(spec.Name, payload)
		if groupExists {
			return groupId, true, nil
		}

		// Stop if we have consumed all results: either the total is known and
		// we have seen everything, or the page was not full (last page).
		if (searchResponse.XTotalCount > 0 && page*pageSize >= searchResponse.XTotalCount) ||
			int64(len(payload)) < pageSize {
			break
		}

		page++
	}

	return 0, false, nil
}
