// Copyright (C) 2020, 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package services

import (
	vmcontrollerv1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/apis/vmcontroller/v1"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/config"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/constants"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// New creates a new Service for a VMO resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the VMO resource that 'owns' it.
func New(vmo *vmcontrollerv1.VerrazzanoMonitoringInstance, useNodeRoleSelectors bool) ([]*corev1.Service, error) {
	var services []*corev1.Service

	if vmo.Spec.Grafana.Enabled {
		service := createServiceElement(vmo, config.Grafana)
		services = append(services, service)
	}
	if vmo.Spec.Prometheus.Enabled {
		service := createServiceElement(vmo, config.Prometheus)
		services = append(services, service)
	}
	if vmo.Spec.AlertManager.Enabled {
		alertManagerService := createServiceElement(vmo, config.AlertManager)
		services = append(services, alertManagerService)

		alertManagerClusterService := createServiceElement(vmo, config.AlertManagerCluster)
		alertManagerClusterService.Spec.Selector = resources.GetSpecID(vmo.Name, config.AlertManager.Name)
		alertManagerClusterService.Spec.Type = corev1.ServiceTypeClusterIP
		alertManagerClusterService.Spec.ClusterIP = corev1.ClusterIPNone
		services = append(services, alertManagerClusterService)
	}
	if vmo.Spec.Elasticsearch.Enabled {
		services = append(services, createOpenSearchServiceElements(vmo, useNodeRoleSelectors)...)
	}
	if vmo.Spec.Kibana.Enabled {
		service := createServiceElement(vmo, config.Kibana)
		services = append(services, service)
	}
	if !config.API.Disabled {
		services = append(services, createServiceElement(vmo, config.API))
	}

	return services, nil
}
func createServiceElement(vmo *vmcontrollerv1.VerrazzanoMonitoringInstance, componentDetails config.ComponentDetails) *corev1.Service {
	resourceLabel := resources.GetMetaLabels(vmo)
	resourceLabel[constants.ComponentLabel] = resources.GetCompLabel(componentDetails.Name)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          resourceLabel,
			Name:            resources.GetMetaName(vmo.Name, componentDetails.Name),
			Namespace:       vmo.Namespace,
			OwnerReferences: resources.GetOwnerReferences(vmo),
		},
		Spec: corev1.ServiceSpec{
			Type:     vmo.Spec.ServiceType,
			Selector: resources.GetSpecID(vmo.Name, componentDetails.Name),
			Ports:    []corev1.ServicePort{resources.GetServicePort(componentDetails)},
		},
	}
}
