/*
Copyright 2024 Clay.

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
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/clay-wangzhi/kube-on-kube/api/kubeonkube/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeClusterOperations implements ClusterOperationInterface
type FakeClusterOperations struct {
	Fake *FakeKubeonkubeV1alpha1
}

var clusteroperationsResource = schema.GroupVersionResource{Group: "kubeonkube.clay.io", Version: "v1alpha1", Resource: "clusteroperations"}

var clusteroperationsKind = schema.GroupVersionKind{Group: "kubeonkube.clay.io", Version: "v1alpha1", Kind: "ClusterOperation"}

// Get takes name of the clusterOperation, and returns the corresponding clusterOperation object, and an error if there is any.
func (c *FakeClusterOperations) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ClusterOperation, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(clusteroperationsResource, name), &v1alpha1.ClusterOperation{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterOperation), err
}

// List takes label and field selectors, and returns the list of ClusterOperations that match those selectors.
func (c *FakeClusterOperations) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ClusterOperationList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(clusteroperationsResource, clusteroperationsKind, opts), &v1alpha1.ClusterOperationList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ClusterOperationList{ListMeta: obj.(*v1alpha1.ClusterOperationList).ListMeta}
	for _, item := range obj.(*v1alpha1.ClusterOperationList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested clusterOperations.
func (c *FakeClusterOperations) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(clusteroperationsResource, opts))
}

// Create takes the representation of a clusterOperation and creates it.  Returns the server's representation of the clusterOperation, and an error, if there is any.
func (c *FakeClusterOperations) Create(ctx context.Context, clusterOperation *v1alpha1.ClusterOperation, opts v1.CreateOptions) (result *v1alpha1.ClusterOperation, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(clusteroperationsResource, clusterOperation), &v1alpha1.ClusterOperation{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterOperation), err
}

// Update takes the representation of a clusterOperation and updates it. Returns the server's representation of the clusterOperation, and an error, if there is any.
func (c *FakeClusterOperations) Update(ctx context.Context, clusterOperation *v1alpha1.ClusterOperation, opts v1.UpdateOptions) (result *v1alpha1.ClusterOperation, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(clusteroperationsResource, clusterOperation), &v1alpha1.ClusterOperation{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterOperation), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeClusterOperations) UpdateStatus(ctx context.Context, clusterOperation *v1alpha1.ClusterOperation, opts v1.UpdateOptions) (*v1alpha1.ClusterOperation, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(clusteroperationsResource, "status", clusterOperation), &v1alpha1.ClusterOperation{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterOperation), err
}

// Delete takes name of the clusterOperation and deletes it. Returns an error if one occurs.
func (c *FakeClusterOperations) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(clusteroperationsResource, name, opts), &v1alpha1.ClusterOperation{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeClusterOperations) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(clusteroperationsResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.ClusterOperationList{})
	return err
}

// Patch applies the patch and returns the patched clusterOperation.
func (c *FakeClusterOperations) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClusterOperation, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(clusteroperationsResource, name, pt, data, subresources...), &v1alpha1.ClusterOperation{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterOperation), err
}
