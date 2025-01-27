// Copyright (C) 2020, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package vmo

import (
	"bytes"
	"context"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/config"
	"html/template"
	"reflect"
	"strings"

	"github.com/verrazzano/pkg/diff"
	vmcontrollerv1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/apis/vmcontroller/v1"

	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/constants"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/resources"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/resources/configmaps"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// CreateConfigmaps to create all required configmaps for VMI
func CreateConfigmaps(controller *Controller, vmo *vmcontrollerv1.VerrazzanoMonitoringInstance) error {
	var configMaps []string
	alertrulesMap := make(map[string]string)

	// Configmap for Grafana dashboard
	dashboardTemplateMap := map[string]string{"vmo-dashboard-provider.yml": constants.DashboardProviderTmpl}
	// Only create the CM if it doesnt exist. This will allow us to override the provider file e.g. Verrazzano
	err := createConfigMapIfDoesntExist(controller, vmo, vmo.Spec.Grafana.DashboardsConfigMap, dashboardTemplateMap)
	if err != nil {
		controller.log.Errorf("Failed to create dashboard configmap %s: %v", vmo.Spec.Grafana.DashboardsConfigMap, err)
		return err

	}
	configMaps = append(configMaps, vmo.Spec.Grafana.DashboardsConfigMap)

	//configmap for grafana datasources
	replaceMap := map[string]string{constants.GrafanaTmplPrometheusURI: resources.GetMetaName(vmo.Name, config.Prometheus.Name),
		constants.GrafanaTmplAlertManagerURI: resources.GetMetaName(vmo.Name, config.AlertManager.Name)}
	dataSourceTemplate, err := asDashboardTemplate(constants.DataSourcesTmpl, replaceMap)
	if err != nil {
		controller.log.Errorf("Failed to create dashboard datasource template for VMI %s: %v", vmo.Name, err)
		return err
	}
	err = createConfigMapIfDoesntExist(controller, vmo, vmo.Spec.Grafana.DatasourcesConfigMap, map[string]string{"datasource.yaml": dataSourceTemplate})
	if err != nil {
		controller.log.Errorf("Failed to create datasource configmap %s: %v", vmo.Spec.Grafana.DatasourcesConfigMap, err)
		return err

	}
	configMaps = append(configMaps, vmo.Spec.Grafana.DatasourcesConfigMap)

	//configmap for alertmanager config
	err = createAMConfigMapIfDoesntExist(controller, vmo, vmo.Spec.AlertManager.ConfigMap, map[string]string{constants.AlertManagerYaml: resources.GetDefaultAlertManagerConfiguration(vmo)})
	if err != nil {
		controller.log.Errorf("Failed to create configmap %s: %v", vmo.Spec.AlertManager.ConfigMap, err)
		return err
	}
	configMaps = append(configMaps, vmo.Spec.AlertManager.ConfigMap)

	//configmap for alertmanager config versions
	//starts off with an empty configmap - Cirith will add to it later
	err = createConfigMapIfDoesntExist(controller, vmo, vmo.Spec.AlertManager.VersionsConfigMap, map[string]string{})
	if err != nil {
		controller.log.Errorf("Failed to create configmap %s: %v", vmo.Spec.AlertManager.VersionsConfigMap, err)
		return err
	}
	configMaps = append(configMaps, vmo.Spec.AlertManager.VersionsConfigMap)

	//configmap for alertrules
	err = createUpdateAlertRulesConfigMap(controller, vmo, vmo.Spec.Prometheus.RulesConfigMap, alertrulesMap)
	if err != nil {
		controller.log.Errorf("Failed to create alertrules configmap %s: %v", vmo.Spec.Prometheus.RulesConfigMap, err)
		return err

	}
	configMaps = append(configMaps, vmo.Spec.Prometheus.RulesConfigMap)

	//configmap for alertrules versions
	//starts off with an empty configmap - Cirith will add to it later
	err = createConfigMapIfDoesntExist(controller, vmo, vmo.Spec.Prometheus.RulesVersionsConfigMap, map[string]string{})
	if err != nil {
		controller.log.Errorf("Failed to create alertrules configmap %s: %v", vmo.Spec.Prometheus.RulesVersionsConfigMap, err)
		return err

	}
	configMaps = append(configMaps, vmo.Spec.Prometheus.RulesVersionsConfigMap)

	//configmap for prometheus config
	vzClusterName := controller.clusterInfo.clusterName
	if vzClusterName == "" {
		vzClusterName, _ = GetClusterNameFromSecret(controller, vmo.Namespace)
	}

	err = reconcilePrometheusConfigMap(controller, vmo, vmo.Spec.Prometheus.ConfigMap, map[string]string{"prometheus.yml": resources.GetDefaultPrometheusConfiguration(vmo, vzClusterName)})
	if err != nil {
		return err
	}
	configMaps = append(configMaps, vmo.Spec.Prometheus.ConfigMap)

	//configmap for prometheus config versions
	//starts off with an empty configmap - Cirith will add to it later.
	err = createConfigMapIfDoesntExist(controller, vmo, vmo.Spec.Prometheus.VersionsConfigMap, map[string]string{})
	if err != nil {
		return err

	}
	configMaps = append(configMaps, vmo.Spec.Prometheus.VersionsConfigMap)

	// Delete configmaps that shouldn't exist
	controller.log.Debugf("Deleting unwanted ConfigMaps for VMI %s/%s", vmo.Namespace, vmo.Name)
	selector := labels.SelectorFromSet(map[string]string{constants.VMOLabel: vmo.Name})
	configMapList, err := controller.configMapLister.ConfigMaps(vmo.Namespace).List(selector)
	if err != nil {
		return err
	}
	for _, configMap := range configMapList {
		if !contains(configMaps, configMap.Name) {
			controller.log.Debugf("Deleting config map %s", configMap.Name)
			err := controller.kubeclientset.CoreV1().ConfigMaps(vmo.Namespace).Delete(context.TODO(), configMap.Name, metav1.DeleteOptions{})
			if err != nil {
				controller.log.Errorf("Failed to delete configmap %s%s: %v", vmo.Namespace, configMap.Name, err)
				return err
			}
		}
	}
	return nil
}

// This function is being called for configmaps which gets modified with spec changes
func createUpdateAlertRulesConfigMap(controller *Controller, vmo *vmcontrollerv1.VerrazzanoMonitoringInstance, configmap string, data map[string]string) error {
	configMap := configmaps.NewConfig(vmo, configmap, data)
	existingConfigMap, err := getConfigMap(controller, vmo.Namespace, configmap)
	if err != nil {
		controller.log.Errorf("Failed to get configmap %s%s: %v", vmo.Namespace, configmap, err)
		return err
	}
	if existingConfigMap != nil {
		controller.log.Debugf("Updating existing configmaps for %s ", existingConfigMap.Name)
		//Retain any AlertManager rules added or modified by user
		if existingConfigMap.Name == resources.GetMetaName(vmo.Name, constants.AlertrulesConfig) {
			//get custom rules if any
			customRules := getCustomRulesMap(existingConfigMap.Data)
			for k, v := range customRules {
				configMap.Data[k] = v
			}
		}
		specDiffs := diff.Diff(existingConfigMap, configMap)
		if specDiffs != "" {
			controller.log.Debugf("ConfigMap %s : Spec differences %s", configMap.Name, specDiffs)
			_, err := controller.kubeclientset.CoreV1().ConfigMaps(vmo.Namespace).Update(context.TODO(), configMap, metav1.UpdateOptions{})
			if err != nil {
				controller.log.Errorf("Failed to update existing configmap %s%s: %v", vmo.Namespace, configmap, err)
			}
		}
	} else {
		_, err := controller.kubeclientset.CoreV1().ConfigMaps(vmo.Namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		if err != nil {
			controller.log.Errorf("Failed to create configmap %s%s: %v", vmo.Namespace, configmap, err)
			return err
		}
	}
	return nil
}

// This function is being called for configmaps which don't modify with spec changes
func createConfigMapIfDoesntExist(controller *Controller, vmo *vmcontrollerv1.VerrazzanoMonitoringInstance, configmap string, data map[string]string) error {
	existingConfig, err := getConfigMap(controller, vmo.Namespace, configmap)
	if err != nil {
		controller.log.Errorf("Failed to get configmap %s%s: %v", vmo.Namespace, configmap, err)
		return err
	}
	if existingConfig == nil {
		configMap := configmaps.NewConfig(vmo, configmap, data)
		_, err := controller.kubeclientset.CoreV1().ConfigMaps(vmo.Namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		if err != nil {
			controller.log.Errorf("Failed to create configmap %s%s: %v", vmo.Namespace, configmap, err)
			return err
		}
	}
	return nil
}

// This function is being called for configmaps which don't modify with spec changes
func createAMConfigMapIfDoesntExist(controller *Controller, vmo *vmcontrollerv1.VerrazzanoMonitoringInstance, configmap string, data map[string]string) error {
	existingConfig, err := getConfigMap(controller, vmo.Namespace, configmap)
	if err != nil {
		controller.log.Errorf("Failed to get configmap %s%s: %v", vmo.Namespace, configmap, err)
		return err
	}
	if existingConfig == nil {
		if vmo.Spec.AlertManager.Config != "" {
			data = map[string]string{constants.AlertManagerYaml: vmo.Spec.AlertManager.Config}
			vmo.Spec.AlertManager.Config = ""
		}
		configMap := configmaps.NewConfig(vmo, configmap, data)
		_, err := controller.kubeclientset.CoreV1().ConfigMaps(vmo.Namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		if err != nil {
			controller.log.Errorf("Failed to create configmap %s%s: %v", vmo.Namespace, configmap, err)
			return err
		}
	}
	return nil
}

//Returns custom-rules in configmap
func getCustomRulesMap(existedData map[string]string) (customRulesMap map[string]string) {
	customRulesMap = make(map[string]string)
	for k, v := range existedData {
		//all default alert rules name starts with vmo-1
		if !strings.HasPrefix(k, constants.VMOServiceNamePrefix) {
			customRulesMap[k] = v
		}
	}
	return
}

// asDashboardTemplate replaces `namespace` placehoders in the tmplt with the namespace value
func asDashboardTemplate(tmplt string, replaceMap map[string]string) (string, error) {
	t := template.Must(template.New("dashboard").Parse(tmplt))
	buf := &bytes.Buffer{}
	if err := t.Execute(buf, replaceMap); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func getConfigMap(controller *Controller, namespace string, configmapName string) (*corev1.ConfigMap, error) {
	configMap, err := controller.configMapLister.ConfigMaps(namespace).Get(configmapName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return configMap, nil
}

// reconcilePrometheusConfigMap reconciles the prometheus configmap data between restarts/upgrades
func reconcilePrometheusConfigMap(controller *Controller, vmo *vmcontrollerv1.VerrazzanoMonitoringInstance, configmap string, data map[string]string) error {
	existingConfig, err := getConfigMap(controller, vmo.Namespace, configmap)
	if err != nil {
		controller.log.Errorf("Failed to get configmap %s%s: %v", vmo.Namespace, configmap, err)
		return err
	}

	if existingConfig == nil {
		configMap := configmaps.NewConfig(vmo, configmap, data)
		_, err := controller.kubeclientset.CoreV1().ConfigMaps(vmo.Namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		if err != nil {
			controller.log.Errorf("Failed to create configmap %s%s: %v", vmo.Namespace, configmap, err)
			return err
		}
		return nil
	}

	if prometheusConfig, ok := existingConfig.Data["prometheus.yml"]; ok {
		var existingConfigYaml map[interface{}]interface{}
		err := yaml.Unmarshal([]byte(prometheusConfig), &existingConfigYaml)
		if err != nil {
			controller.log.Errorf("Failed to read configmap %s%s: %v", vmo.Namespace, configmap, err)
			return err
		}

		var existingScrapeConfigs []interface{}
		if existingScrapeConfigsData, ok := existingConfigYaml["scrape_configs"]; ok {
			var newConfig map[interface{}]interface{}
			err := yaml.Unmarshal([]byte(data["prometheus.yml"]), &newConfig)
			if err != nil {
				controller.log.Errorf("Failed to read new configmap data: %v", err)
				return err
			}

			var scrapeConfigs []interface{}
			var newScrapeConfigs []interface{}
			var scrapeConfigChanged bool
			if newScrapeConfigsData, ok := newConfig["scrape_configs"]; ok {
				newScrapeConfigs = newScrapeConfigsData.([]interface{})
				existingScrapeConfigs = existingScrapeConfigsData.([]interface{})
				for _, esc := range existingScrapeConfigs {
					existingScrapeConfig := esc.(map[interface{}]interface{})
					found := false
					for _, nsc := range newScrapeConfigs {
						newScrapeConfig := nsc.(map[interface{}]interface{})
						if newScrapeConfig["job_name"] == existingScrapeConfig["job_name"] {
							// If the existing scrape config of a job is different than default, revert to default
							if !reflect.DeepEqual(newScrapeConfig, existingScrapeConfig) {
								scrapeConfigChanged = true
								scrapeConfigs = append(scrapeConfigs, newScrapeConfig)
							} else {
								scrapeConfigs = append(scrapeConfigs, existingScrapeConfig)
							}

							found = true
							break
						}
					}
					// Preserve all scrape configs that are not part of default config
					if !found {
						scrapeConfigs = append(scrapeConfigs, existingScrapeConfig)
					}
				}

				// Add all default configs that do not exist
				for _, nsc := range newScrapeConfigs {
					newScrapeConfig := nsc.(map[interface{}]interface{})
					found := false
					for _, sc := range scrapeConfigs {
						scrapeConfig := sc.(map[interface{}]interface{})
						if newScrapeConfig["job_name"] == scrapeConfig["job_name"] {
							found = true
							break
						}
					}
					if !found {
						scrapeConfigChanged = true
						scrapeConfigs = append(scrapeConfigs, newScrapeConfig)
					}
				}
			}

			// Update the configmap only when there is a change to scrap config data
			if len(scrapeConfigs) > 0 && (len(scrapeConfigs) != len(existingScrapeConfigs) || scrapeConfigChanged) {
				newConfig["scrape_configs"] = scrapeConfigs
				newConfigYaml, err := yaml.Marshal(&newConfig)
				if err != nil {
					controller.log.Errorf("Failed to update new configmap data: %v", err)
					return err
				}
				data["prometheus.yml"] = string(newConfigYaml)
				existingConfig.Data = data
				_, err = controller.kubeclientset.CoreV1().ConfigMaps(vmo.Namespace).Update(context.TODO(), existingConfig, metav1.UpdateOptions{})
				if err != nil {
					controller.log.Errorf("Failed to update configmap %s%s: %v", vmo.Namespace, existingConfig, err)
					return err
				}

				return nil
			}
		}
	}
	return nil
}
