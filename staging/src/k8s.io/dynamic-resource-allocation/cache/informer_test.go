/*
Copyright 2023 The Kubernetes Authors.

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

package cache

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/cached/memory"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/ktesting"
)

var secrets = []v1.Secret{
	{ObjectMeta: metav1.ObjectMeta{Name: "foo1", Namespace: "namespace1"}},
	{ObjectMeta: metav1.ObjectMeta{Name: "foo2", Namespace: "namespace2"}},
}

func TestAddRemove(t *testing.T) {
	ctx, factory := setup(t)
	gk := schema.GroupKind{Kind: "Secret"}
	listerCtx := klog.NewContext(ctx, klog.LoggerWithName(klog.FromContext(ctx), "SecretTester"))
	listerCtx, cancel := context.WithCancel(listerCtx)
	defer cancel()
	factory.ForType(listerCtx, gk)
}

func TestList(t *testing.T) {
	ctx, factory := setup(t)
	gk := schema.GroupKind{Kind: "Secret"}
	for i := 0; i < 5; i++ {
		listerCtx := klog.NewContext(ctx, klog.LoggerWithName(klog.FromContext(ctx), fmt.Sprintf("SecretTester_%d", i)))
		listerCtx, cancel := context.WithCancel(listerCtx)
		defer cancel()
		lister := factory.ForType(listerCtx, gk)
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			all, err := lister.List(labels.Everything())
			require.NoError(collect, err, "list all secrets")
			require.Equal(t, []string{secrets[0].Name, secrets[1].Name}, names(all), "all secrets")
			one, err := lister.ByNamespace(secrets[0].Namespace).List(labels.Everything())
			require.NoError(collect, err, "list one secret")
			require.Equal(t, []string{secrets[0].Name}, names(one), "one secret")
		}, 10*time.Second, time.Second)
	}
}

func setup(t *testing.T) (context.Context, *GenericListerFactory) {
	_, ctx := ktesting.NewTestContext(t)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	client := fakedynamic.NewSimpleDynamicClient(scheme, &secrets[0], &secrets[1])
	discovery := &fakediscovery.FakeDiscovery{Fake: &client.Fake}
	discovery.Resources = append(discovery.Resources,
		&metav1.APIResourceList{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "secrets",
					SingularName: "secret",
					Namespaced:   true,
					Version:      "v1",
					Kind:         "Secret",
				},
			},
		},
	)
	cachedDiscovery := memory.NewMemCacheClient(discovery)
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscovery)
	factory := StartGenericListerFactory(ctx, client, restMapper)
	t.Cleanup(factory.Shutdown)
	return ctx, factory
}

func names(objects []runtime.Object) []string {
	result := make([]string, 0, len(objects))
	for _, object := range objects {
		result = append(result, object.(*unstructured.Unstructured).GetName())
	}
	sort.Strings(result)
	return result
}
