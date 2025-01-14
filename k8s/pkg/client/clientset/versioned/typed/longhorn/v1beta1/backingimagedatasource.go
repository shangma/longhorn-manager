/*
Copyright The Kubernetes Authors.

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

package v1beta1

import (
	"time"

	v1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	scheme "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// BackingImageDataSourcesGetter has a method to return a BackingImageDataSourceInterface.
// A group's client should implement this interface.
type BackingImageDataSourcesGetter interface {
	BackingImageDataSources(namespace string) BackingImageDataSourceInterface
}

// BackingImageDataSourceInterface has methods to work with BackingImageDataSource resources.
type BackingImageDataSourceInterface interface {
	Create(*v1beta1.BackingImageDataSource) (*v1beta1.BackingImageDataSource, error)
	Update(*v1beta1.BackingImageDataSource) (*v1beta1.BackingImageDataSource, error)
	UpdateStatus(*v1beta1.BackingImageDataSource) (*v1beta1.BackingImageDataSource, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1beta1.BackingImageDataSource, error)
	List(opts v1.ListOptions) (*v1beta1.BackingImageDataSourceList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.BackingImageDataSource, err error)
	BackingImageDataSourceExpansion
}

// backingImageDataSources implements BackingImageDataSourceInterface
type backingImageDataSources struct {
	client rest.Interface
	ns     string
}

// newBackingImageDataSources returns a BackingImageDataSources
func newBackingImageDataSources(c *LonghornV1beta1Client, namespace string) *backingImageDataSources {
	return &backingImageDataSources{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the backingImageDataSource, and returns the corresponding backingImageDataSource object, and an error if there is any.
func (c *backingImageDataSources) Get(name string, options v1.GetOptions) (result *v1beta1.BackingImageDataSource, err error) {
	result = &v1beta1.BackingImageDataSource{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("backingimagedatasources").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of BackingImageDataSources that match those selectors.
func (c *backingImageDataSources) List(opts v1.ListOptions) (result *v1beta1.BackingImageDataSourceList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1beta1.BackingImageDataSourceList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("backingimagedatasources").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested backingImageDataSources.
func (c *backingImageDataSources) Watch(opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("backingimagedatasources").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch()
}

// Create takes the representation of a backingImageDataSource and creates it.  Returns the server's representation of the backingImageDataSource, and an error, if there is any.
func (c *backingImageDataSources) Create(backingImageDataSource *v1beta1.BackingImageDataSource) (result *v1beta1.BackingImageDataSource, err error) {
	result = &v1beta1.BackingImageDataSource{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("backingimagedatasources").
		Body(backingImageDataSource).
		Do().
		Into(result)
	return
}

// Update takes the representation of a backingImageDataSource and updates it. Returns the server's representation of the backingImageDataSource, and an error, if there is any.
func (c *backingImageDataSources) Update(backingImageDataSource *v1beta1.BackingImageDataSource) (result *v1beta1.BackingImageDataSource, err error) {
	result = &v1beta1.BackingImageDataSource{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("backingimagedatasources").
		Name(backingImageDataSource.Name).
		Body(backingImageDataSource).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *backingImageDataSources) UpdateStatus(backingImageDataSource *v1beta1.BackingImageDataSource) (result *v1beta1.BackingImageDataSource, err error) {
	result = &v1beta1.BackingImageDataSource{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("backingimagedatasources").
		Name(backingImageDataSource.Name).
		SubResource("status").
		Body(backingImageDataSource).
		Do().
		Into(result)
	return
}

// Delete takes name of the backingImageDataSource and deletes it. Returns an error if one occurs.
func (c *backingImageDataSources) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("backingimagedatasources").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *backingImageDataSources) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("backingimagedatasources").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched backingImageDataSource.
func (c *backingImageDataSources) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.BackingImageDataSource, err error) {
	result = &v1beta1.BackingImageDataSource{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("backingimagedatasources").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
