// Copyright (c) 2020, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/client/clientset/versioned/typed/vmcontroller/v1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeVerrazzanoV1 struct {
	*testing.Fake
}

func (c *FakeVerrazzanoV1) VerrazzanoMonitoringInstances(namespace string) v1.VerrazzanoMonitoringInstanceInterface {
	return &FakeVerrazzanoMonitoringInstances{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeVerrazzanoV1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
