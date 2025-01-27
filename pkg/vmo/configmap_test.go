// Copyright (C) 2020, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package vmo

import (
	"context"
	"testing"

	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/util/logs/vzlog"
	"gopkg.in/yaml.v2"

	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/constants"
	corelistersv1 "k8s.io/client-go/listers/core/v1"

	"github.com/stretchr/testify/assert"
	vmctl "github.com/verrazzano/verrazzano-monitoring-operator/pkg/apis/vmcontroller/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/kubernetes/fake"
)

func TestCreateConfigmaps(t *testing.T) {
	client := fake.NewSimpleClientset()
	controller := &Controller{
		kubeclientset:   client,
		configMapLister: &simpleConfigMapLister{kubeClient: client},
		secretLister:    &simpleSecretLister{kubeClient: client},
		log:             vzlog.DefaultLogger(),
	}
	vmo := &vmctl.VerrazzanoMonitoringInstance{}
	vmo.Name = constants.VMODefaultName
	vmo.Namespace = constants.VerrazzanoSystemNamespace
	vmo.Spec.URI = "vmi.system.v8o-env.oracledx.com"
	vmo.Spec.Grafana.DashboardsConfigMap = "myDashboardsConfigMap"
	vmo.Spec.Grafana.DatasourcesConfigMap = "myDatasourcesConfigMap"
	vmo.Spec.AlertManager.ConfigMap = "myAlertManagerConfigMap"
	vmo.Spec.AlertManager.VersionsConfigMap = "myAlertManagerVersionsConfigMap"
	vmo.Spec.Prometheus.RulesConfigMap = "myPrometheusRulesConfigMap"
	vmo.Spec.Prometheus.RulesVersionsConfigMap = "myPrometheusRulesVersionsConfigMap"
	vmo.Spec.Prometheus.ConfigMap = "myPrometheusConfigMap"
	vmo.Spec.Prometheus.VersionsConfigMap = "myPrometheusVersionsConfigMap"
	err := CreateConfigmaps(controller, vmo)
	t.Logf("Error is %v", err)
	assert.Nil(t, err)
	all, _ := client.CoreV1().ConfigMaps(vmo.Namespace).List(context.TODO(), metav1.ListOptions{})
	assert.Equal(t, 8, len(all.Items))
}

// simple ConfigMapLister implementation
type simpleConfigMapLister struct {
	kubeClient kubernetes.Interface
}

// lists all ConfigMaps
func (s *simpleConfigMapLister) List(selector labels.Selector) ([]*v1.ConfigMap, error) {
	namespaces, err := s.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var pods []*v1.ConfigMap
	for _, namespace := range namespaces.Items {

		list, err := s.ConfigMaps(namespace.Name).List(selector)
		if err != nil {
			return nil, err
		}
		pods = append(pods, list...)
	}
	return pods, nil
}

// ConfigMaps returns an object that can list and get ConfigMaps.
func (s *simpleConfigMapLister) ConfigMaps(namespace string) corelistersv1.ConfigMapNamespaceLister {
	return simpleConfigMapNamespaceLister{
		namespace:  namespace,
		kubeClient: s.kubeClient,
	}
}

// configMapNamespaceLister implements the ConfigMapNamespaceLister
// interface.
type simpleConfigMapNamespaceLister struct {
	namespace  string
	kubeClient kubernetes.Interface
}

// List lists all ConfigMaps for a given namespace.
func (s simpleConfigMapNamespaceLister) List(selector labels.Selector) ([]*v1.ConfigMap, error) {
	var configMaps []*v1.ConfigMap

	list, err := s.kubeClient.CoreV1().ConfigMaps(s.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range list.Items {
		if selector.Matches(labels.Set(list.Items[i].Labels)) {
			configMaps = append(configMaps, &list.Items[i])
		}
	}
	return configMaps, nil
}

// Get retrieves the ConfigMap for a given namespace and name.
func (s simpleConfigMapNamespaceLister) Get(name string) (*v1.ConfigMap, error) {
	return s.kubeClient.CoreV1().ConfigMaps(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// simple SecretLister implementation
type simpleSecretLister struct {
	kubeClient kubernetes.Interface
}

// lists all Secrets
func (s *simpleSecretLister) List(selector labels.Selector) ([]*v1.Secret, error) {
	namespaces, err := s.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var secrets []*v1.Secret
	for _, namespace := range namespaces.Items {

		list, err := s.Secrets(namespace.Name).List(selector)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, list...)
	}
	return secrets, nil
}

// Secrets returns an object that can get Secrets.
func (s *simpleSecretLister) Secrets(namespace string) corelistersv1.SecretNamespaceLister {
	return simpleSecretNamespaceLister{
		namespace:  namespace,
		kubeClient: s.kubeClient,
	}
}

// simpleSecretNamespaceLister implements the SecretNamespaceLister
// interface.
type simpleSecretNamespaceLister struct {
	namespace  string
	kubeClient kubernetes.Interface
}

// List lists all Secrets for a given namespace.
func (s simpleSecretNamespaceLister) List(selector labels.Selector) ([]*v1.Secret, error) {
	var secrets []*v1.Secret

	list, err := s.kubeClient.CoreV1().Secrets(s.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range list.Items {
		if selector.Matches(labels.Set(list.Items[i].Labels)) {
			secrets = append(secrets, &list.Items[i])
		}
	}
	return secrets, nil
}

// Get retrieves the Secret for a given namespace and name.
func (s simpleSecretNamespaceLister) Get(name string) (*v1.Secret, error) {
	return s.kubeClient.CoreV1().Secrets(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

//TestReconcileConfigmapsDefaultScrapeConfigsRestoredAfterReconcile tests that any changes to default scrape configs will be restored after reconcile
func TestReconcileConfigmapsDefaultScrapeConfigsRestoredAfterReconcile(t *testing.T) {
	client := fake.NewSimpleClientset()
	controller := &Controller{
		kubeclientset:   client,
		configMapLister: &simpleConfigMapLister{kubeClient: client},
		secretLister:    &simpleSecretLister{kubeClient: client},
		log:             vzlog.DefaultLogger(),
	}
	vmo := &vmctl.VerrazzanoMonitoringInstance{}
	vmo.Name = constants.VMODefaultName
	vmo.Namespace = constants.VerrazzanoSystemNamespace
	vmo.Spec.URI = "vmi.system.v8o-env.oracledx.com"
	vmo.Spec.Prometheus.ConfigMap = "myPrometheusConfigMap"

	err := CreateConfigmaps(controller, vmo)
	t.Logf("Error is %v", err)
	assert.Nil(t, err)
	cm, _ := client.CoreV1().ConfigMaps(vmo.Namespace).Get(context.TODO(), vmo.Spec.Prometheus.ConfigMap, metav1.GetOptions{})
	prometheusConfig, ok := cm.Data["prometheus.yml"]
	assert.True(t, ok)
	var configYaml map[interface{}]interface{}
	err = yaml.Unmarshal([]byte(prometheusConfig), &configYaml)
	assert.NoError(t, err)
	scrapeConfigsData, ok := configYaml["scrape_configs"]
	assert.True(t, ok)
	scrapeConfigs := scrapeConfigsData.([]interface{})
	var originalScrapeInterval string
	for _, nsc := range scrapeConfigs {
		scrapeConfig := nsc.(map[interface{}]interface{})
		if scrapeConfig["job_name"] == "prometheus" {
			originalScrapeInterval = scrapeConfig["scrape_interval"].(string)
			scrapeConfig["scrape_interval"] = "10000000s"
			break
		}
	}

	configYaml["scrape_configs"] = scrapeConfigs
	newConfigYaml, err := yaml.Marshal(&configYaml)
	assert.NoError(t, err)
	cm.Data["prometheus.yml"] = string(newConfigYaml)
	_, err = client.CoreV1().ConfigMaps(vmo.Namespace).Update(context.TODO(), cm, metav1.UpdateOptions{})
	assert.NoError(t, err)

	err = CreateConfigmaps(controller, vmo)
	t.Logf("Error is %v", err)
	assert.Nil(t, err)
	cm, _ = client.CoreV1().ConfigMaps(vmo.Namespace).Get(context.TODO(), vmo.Spec.Prometheus.ConfigMap, metav1.GetOptions{})
	prometheusConfig, ok = cm.Data["prometheus.yml"]
	assert.True(t, ok)
	err = yaml.Unmarshal([]byte(prometheusConfig), &configYaml)
	assert.NoError(t, err)
	scrapeConfigsData, ok = configYaml["scrape_configs"]
	assert.True(t, ok)
	scrapeConfigs = scrapeConfigsData.([]interface{})
	found := false
	var afterReconcileScrapeInterval string
	for _, nsc := range scrapeConfigs {
		scrapeConfig := nsc.(map[interface{}]interface{})
		if scrapeConfig["job_name"] == "prometheus" {
			found = true
			afterReconcileScrapeInterval = scrapeConfig["scrape_interval"].(string)
			break
		}
	}
	assert.True(t, found)
	assert.Equal(t, originalScrapeInterval, afterReconcileScrapeInterval)
}

//TestReconcileConfigmapsNewScrapeConfigsIntactAfterReconcile tests that any new scrape configs will be intact after reconcile
func TestReconcileConfigmapsNewScrapeConfigsIntactAfterReconcile(t *testing.T) {
	client := fake.NewSimpleClientset()
	controller := &Controller{
		kubeclientset:   client,
		configMapLister: &simpleConfigMapLister{kubeClient: client},
		secretLister:    &simpleSecretLister{kubeClient: client},
		log:             vzlog.DefaultLogger(),
	}
	vmo := &vmctl.VerrazzanoMonitoringInstance{}
	vmo.Name = constants.VMODefaultName
	vmo.Namespace = constants.VerrazzanoSystemNamespace
	vmo.Spec.URI = "vmi.system.v8o-env.oracledx.com"
	vmo.Spec.Prometheus.ConfigMap = "myPrometheusConfigMap"

	err := CreateConfigmaps(controller, vmo)
	t.Logf("Error is %v", err)
	assert.Nil(t, err)
	cm, _ := client.CoreV1().ConfigMaps(vmo.Namespace).Get(context.TODO(), vmo.Spec.Prometheus.ConfigMap, metav1.GetOptions{})
	prometheusConfig, ok := cm.Data["prometheus.yml"]
	assert.True(t, ok)
	var configYaml map[interface{}]interface{}
	err = yaml.Unmarshal([]byte(prometheusConfig), &configYaml)
	assert.NoError(t, err)
	scrapeConfigsData, ok := configYaml["scrape_configs"]
	assert.True(t, ok)
	scrapeConfigs := scrapeConfigsData.([]interface{})

	dummyScrapConfig := make(map[interface{}]interface{})
	dummyScrapConfig["job_name"] = "dummy_job"
	scrapeConfigs = append(scrapeConfigs, dummyScrapConfig)
	configYaml["scrape_configs"] = scrapeConfigs
	newConfigYaml, err := yaml.Marshal(&configYaml)
	assert.NoError(t, err)
	cm.Data["prometheus.yml"] = string(newConfigYaml)
	_, err = client.CoreV1().ConfigMaps(vmo.Namespace).Update(context.TODO(), cm, metav1.UpdateOptions{})
	assert.NoError(t, err)

	err = CreateConfigmaps(controller, vmo)
	t.Logf("Error is %v", err)
	assert.Nil(t, err)
	cm, _ = client.CoreV1().ConfigMaps(vmo.Namespace).Get(context.TODO(), vmo.Spec.Prometheus.ConfigMap, metav1.GetOptions{})
	prometheusConfig, ok = cm.Data["prometheus.yml"]
	assert.True(t, ok)
	err = yaml.Unmarshal([]byte(prometheusConfig), &configYaml)
	assert.NoError(t, err)
	scrapeConfigsData, ok = configYaml["scrape_configs"]
	assert.True(t, ok)
	scrapeConfigs = scrapeConfigsData.([]interface{})
	found := false
	for _, nsc := range scrapeConfigs {
		scrapeConfig := nsc.(map[interface{}]interface{})
		if scrapeConfig["job_name"] == "dummy_job" {
			found = true
			break
		}
	}
	assert.True(t, found)
}
