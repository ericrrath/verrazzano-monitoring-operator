// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	"context"
	time "time"

	vmcontrollerv1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/apis/vmcontroller/v1"
	versioned "github.com/verrazzano/verrazzano-monitoring-operator/pkg/client/clientset/versioned"
	internalinterfaces "github.com/verrazzano/verrazzano-monitoring-operator/pkg/client/informers/externalversions/internalinterfaces"
	v1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/client/listers/vmcontroller/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// VerrazzanoMonitoringInstanceInformer provides access to a shared informer and lister for
// VerrazzanoMonitoringInstances.
type VerrazzanoMonitoringInstanceInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.VerrazzanoMonitoringInstanceLister
}

type verrazzanoMonitoringInstanceInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewVerrazzanoMonitoringInstanceInformer constructs a new informer for VerrazzanoMonitoringInstance type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewVerrazzanoMonitoringInstanceInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredVerrazzanoMonitoringInstanceInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredVerrazzanoMonitoringInstanceInformer constructs a new informer for VerrazzanoMonitoringInstance type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredVerrazzanoMonitoringInstanceInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.VerrazzanoV1().VerrazzanoMonitoringInstances(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.VerrazzanoV1().VerrazzanoMonitoringInstances(namespace).Watch(context.TODO(), options)
			},
		},
		&vmcontrollerv1.VerrazzanoMonitoringInstance{},
		resyncPeriod,
		indexers,
	)
}

func (f *verrazzanoMonitoringInstanceInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredVerrazzanoMonitoringInstanceInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *verrazzanoMonitoringInstanceInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&vmcontrollerv1.VerrazzanoMonitoringInstance{}, f.defaultInformer)
}

func (f *verrazzanoMonitoringInstanceInformer) Lister() v1.VerrazzanoMonitoringInstanceLister {
	return v1.NewVerrazzanoMonitoringInstanceLister(f.Informer().GetIndexer())
}
