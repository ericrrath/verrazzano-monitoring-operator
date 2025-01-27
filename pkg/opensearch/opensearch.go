// Copyright (C) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package opensearch

import (
	"fmt"
	vmcontrollerv1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/apis/vmcontroller/v1"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/resources"
	"net/http"
)

type (
	OSClient struct {
		httpClient *http.Client
		DoHTTP     func(request *http.Request) (*http.Response, error)
	}
)

func NewOSClient() *OSClient {
	o := &OSClient{
		httpClient: http.DefaultClient,
	}
	o.DoHTTP = func(request *http.Request) (*http.Response, error) {
		return o.httpClient.Do(request)
	}
	return o
}

//IsDataResizable returns an error unless these conditions of the OpenSearch cluster are met
// - at least 2 data nodes
// - 'green' health
// - all expected nodes are present in the cluster status
func (o *OSClient) IsDataResizable(vmo *vmcontrollerv1.VerrazzanoMonitoringInstance) error {
	if vmo.Spec.Elasticsearch.DataNode.Replicas < MinDataNodesForResize {
		return fmt.Errorf("cannot resize OpenSearch with less than %d data nodes. Scale up your cluster to at least %d data nodes", MinDataNodesForResize, MinDataNodesForResize)
	}
	return o.opensearchHealth(vmo, true, true)
}

//IsUpdated returns an error unless these conditions of the OpenSearch cluster are met
// - 'green' health
// - all expected nodes are present in the cluster status
func (o *OSClient) IsUpdated(vmo *vmcontrollerv1.VerrazzanoMonitoringInstance) error {
	return o.opensearchHealth(vmo, true, true)
}

//IsGreen returns an error unless these conditions of the OpenSearch cluster are met
// - 'green' health
func (o *OSClient) IsGreen(vmo *vmcontrollerv1.VerrazzanoMonitoringInstance) error {
	return o.opensearchHealth(vmo, false, false)
}

//ConfigureISM sets up the ISM Policies
// The returned channel should be read for exactly one response, which tells whether ISM configuration succeeded.
func (o *OSClient) ConfigureISM(vmi *vmcontrollerv1.VerrazzanoMonitoringInstance) chan error {
	ch := make(chan error)
	// configuration is done asynchronously, as this does not need to be blocking
	go func() {
		if !vmi.Spec.Elasticsearch.Enabled {
			ch <- nil
			return
		}

		opensearchEndpoint := resources.GetOpenSearchHTTPEndpoint(vmi)
		for _, policy := range vmi.Spec.Elasticsearch.Policies {
			if err := o.createISMPolicy(opensearchEndpoint, policy); err != nil {
				ch <- err
				return
			}
		}

		ch <- o.cleanupPolicies(opensearchEndpoint, vmi.Spec.Elasticsearch.Policies)
	}()

	return ch
}
