// Copyright (C) 2020, Oracle Corporation and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
package vmo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/rs/zerolog"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslistersv1 "k8s.io/client-go/listers/apps/v1"
	batchlistersv1beta1 "k8s.io/client-go/listers/batch/v1beta1"
	corelistersv1 "k8s.io/client-go/listers/core/v1"
	extensionslistersv1beta1 "k8s.io/client-go/listers/extensions/v1beta1"
	rbacv1listers1 "k8s.io/client-go/listers/rbac/v1"
	storagelisters1 "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	vmcontrollerv1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/apis/vmcontroller/v1"
	clientset "github.com/verrazzano/verrazzano-monitoring-operator/pkg/client/clientset/versioned"
	clientsetscheme "github.com/verrazzano/verrazzano-monitoring-operator/pkg/client/clientset/versioned/scheme"
	informers "github.com/verrazzano/verrazzano-monitoring-operator/pkg/client/informers/externalversions"
	listers "github.com/verrazzano/verrazzano-monitoring-operator/pkg/client/listers/vmcontroller/v1"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/config"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/constants"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/metrics"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/signals"
	"github.com/verrazzano/verrazzano-monitoring-operator/pkg/util/diff"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/tools/clientcmd"
)

const controllerAgentName = "sauron-controller"

// Controller is the controller implementation for Sauron resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sauronclientset is a clientset for our own API group
	sauronclientset  clientset.Interface
	kubeextclientset apiextensionsclient.Interface

	// listers and syncs
	clusterRoleLister    rbacv1listers1.ClusterRoleLister
	clusterRolesSynced   cache.InformerSynced
	configMapLister      corelistersv1.ConfigMapLister
	configMapsSynced     cache.InformerSynced
	cronJobLister        batchlistersv1beta1.CronJobLister
	cronJobsSynced       cache.InformerSynced
	deploymentLister     appslistersv1.DeploymentLister
	deploymentsSynced    cache.InformerSynced
	ingressLister        extensionslistersv1beta1.IngressLister
	ingressesSynced      cache.InformerSynced
	nodeLister           corelistersv1.NodeLister
	nodesSynced          cache.InformerSynced
	pvcLister            corelistersv1.PersistentVolumeClaimLister
	pvcsSynced           cache.InformerSynced
	roleBindingLister    rbacv1listers1.RoleBindingLister
	roleBindingsSynced   cache.InformerSynced
	secretLister         corelistersv1.SecretLister
	secretsSynced        cache.InformerSynced
	serviceLister        corelistersv1.ServiceLister
	servicesSynced       cache.InformerSynced
	statefulSetLister    appslistersv1.StatefulSetLister
	statefulSetsSynced   cache.InformerSynced
	sauronLister         listers.VerrazzanoMonitoringInstanceLister
	sauronsSynced        cache.InformerSynced
	storageClassLister   storagelisters1.StorageClassLister
	storageClassesSynced cache.InformerSynced

	// misc
	namespace      string
	watchNamespace string
	watchVmi       string
	buildVersion   string
	stopCh         <-chan struct{}

	// config
	operatorConfigMapName string
	operatorConfig        *config.OperatorConfig
	latestConfigMap       *corev1.ConfigMap

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// lastEnqueue is the timestamp of when the last element was added to the queue
	lastEnqueue time.Time
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sauron controller
func NewController(namespace string, configmapName string, buildVersion string, kubeconfig string, masterURL string, watchNamespace string, watchVmi string) (*Controller, error) {
	//create log for main function
	logger := zerolog.New(os.Stderr).With().Timestamp().Str("kind", "ClusterOperator").Str("name", "ClusterInit").Logger()

	logger.Debug().Msg("Building config")
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		logger.Fatal().Msgf("Error building kubeconfig: %v", err)
	}

	logger.Debug().Msg("Building kubernetes clientset")
	kubeclientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatal().Msgf("Error building kubernetes clientset: %v", err)
	}

	logger.Debug().Msg("Building sauron clientset")
	sauronclientset, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatal().Msgf("Error building sauron clientset: %v", err)
	}

	logger.Debug().Msg("Building api extensions clientset")
	kubeextclientset, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		logger.Fatal().Msgf("Error building apiextensions-apiserver clientset: %v", err)
	}

	// Get the config from the ConfigMap
	logger.Debug().Msgf("Loading ConfigMap ", configmapName)

	operatorConfigMap, err := kubeclientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configmapName, metav1.GetOptions{})
	if err != nil {
		logger.Fatal().Msgf("No configuration ConfigMap called %s found in namespace %s.", configmapName, namespace)
	}
	logger.Debug().Msgf("Building config from ConfigMap %s", configmapName)
	operatorConfig, err := config.NewConfigFromConfigMap(operatorConfigMap)
	if err != nil {
		logger.Fatal().Msgf("Error building verrazzano-monitoring-operator config from config map: %s", err.Error())
	}

	var kubeInformerFactory kubeinformers.SharedInformerFactory
	var sauronInformerFactory informers.SharedInformerFactory
	if watchNamespace == "" {
		// Consider all namespaces if our namespace is left wide open our set to default
		kubeInformerFactory = kubeinformers.NewSharedInformerFactory(kubeclientset, constants.ResyncPeriod)
		sauronInformerFactory = informers.NewSharedInformerFactory(sauronclientset, constants.ResyncPeriod)
	} else {
		// Otherwise, restrict to a specific namespace
		kubeInformerFactory = kubeinformers.NewSharedInformerFactoryWithOptions(kubeclientset, constants.ResyncPeriod, kubeinformers.WithNamespace(watchNamespace), kubeinformers.WithTweakListOptions(nil))
		sauronInformerFactory = informers.NewSharedInformerFactoryWithOptions(sauronclientset, constants.ResyncPeriod, informers.WithNamespace(watchNamespace), informers.WithTweakListOptions(nil))
	}

	// obtain references to shared index informers for the Deployment and Sauron
	// types.
	clusterRoleInformer := kubeInformerFactory.Rbac().V1().ClusterRoles()
	configmapInformer := kubeInformerFactory.Core().V1().ConfigMaps()
	cronJobInformer := kubeInformerFactory.Batch().V1beta1().CronJobs()
	deploymentInformer := kubeInformerFactory.Apps().V1().Deployments()
	ingressInformer := kubeInformerFactory.Extensions().V1beta1().Ingresses()
	nodeInformer := kubeInformerFactory.Core().V1().Nodes()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	roleBindingInformer := kubeInformerFactory.Rbac().V1().RoleBindings()
	secretsInformer := kubeInformerFactory.Core().V1().Secrets()
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	statefulSetInformer := kubeInformerFactory.Apps().V1().StatefulSets()
	sauronInformer := sauronInformerFactory.Verrazzano().V1().VerrazzanoMonitoringInstances()
	storageClassInformer := kubeInformerFactory.Storage().V1().StorageClasses()
	// Create event broadcaster
	// Add sauron-controller types to the default Kubernetes Scheme so Events can be
	// logged for sauron-controller types.
	if err := clientsetscheme.AddToScheme(scheme.Scheme); err != nil {
		logger.Warn().Msgf("error adding scheme: %+v", err)
	}
	logger.Info().Msg("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logger.Info().Msgf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		namespace:        namespace,
		watchNamespace:   watchNamespace,
		watchVmi:         watchVmi,
		kubeclientset:    kubeclientset,
		sauronclientset:  sauronclientset,
		kubeextclientset: kubeextclientset,

		clusterRoleLister:     clusterRoleInformer.Lister(),
		clusterRolesSynced:    clusterRoleInformer.Informer().HasSynced,
		configMapLister:       configmapInformer.Lister(),
		configMapsSynced:      configmapInformer.Informer().HasSynced,
		cronJobLister:         cronJobInformer.Lister(),
		cronJobsSynced:        cronJobInformer.Informer().HasSynced,
		deploymentLister:      deploymentInformer.Lister(),
		deploymentsSynced:     deploymentInformer.Informer().HasSynced,
		ingressLister:         ingressInformer.Lister(),
		ingressesSynced:       ingressInformer.Informer().HasSynced,
		nodeLister:            nodeInformer.Lister(),
		nodesSynced:           nodeInformer.Informer().HasSynced,
		pvcLister:             pvcInformer.Lister(),
		pvcsSynced:            pvcInformer.Informer().HasSynced,
		roleBindingLister:     roleBindingInformer.Lister(),
		roleBindingsSynced:    roleBindingInformer.Informer().HasSynced,
		secretLister:          secretsInformer.Lister(),
		secretsSynced:         secretsInformer.Informer().HasSynced,
		serviceLister:         serviceInformer.Lister(),
		servicesSynced:        serviceInformer.Informer().HasSynced,
		statefulSetLister:     statefulSetInformer.Lister(),
		statefulSetsSynced:    statefulSetInformer.Informer().HasSynced,
		sauronLister:          sauronInformer.Lister(),
		sauronsSynced:         sauronInformer.Informer().HasSynced,
		storageClassLister:    storageClassInformer.Lister(),
		storageClassesSynced:  storageClassInformer.Informer().HasSynced,
		workqueue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Saurons"),
		recorder:              recorder,
		buildVersion:          buildVersion,
		operatorConfigMapName: configmapName,
		operatorConfig:        operatorConfig,
		latestConfigMap:       operatorConfigMap,
	}

	logger.Info().Msg("Setting up event handlers")

	// Set up an event handler for when Sauron resources change
	sauronInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueSauron,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueSauron(new)
		},
	})

	// Create watchers on the operator ConfigMap, which may signify a need to reload our config
	configMapInformer := kubeInformerFactory.Core().V1().ConfigMaps()
	configMapInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			newConfigMap := new.(*corev1.ConfigMap)
			// If the configMap has changed from our last known copy, process it
			if newConfigMap.Name == controller.operatorConfigMapName && !reflect.DeepEqual(newConfigMap.Data, controller.latestConfigMap.Data) {
				logger.Info().Msg("Reloading config...")
				newOperatorConfig, err := config.NewConfigFromConfigMap(newConfigMap)
				if err != nil {
					logger.Error().Msgf("Errors processing config updates - so we're staying at current configuration: %s", err)
				} else {
					logger.Info().Msg("Successfully reloaded config")
					controller.operatorConfig = newOperatorConfig
					controller.latestConfigMap = newConfigMap
				}
			}
		},
	})

	// set up signals so we handle the first shutdown signal gracefully
	logger.Debug().Msg("Setting up signals")
	controller.stopCh = signals.SetupSignalHandler()

	go kubeInformerFactory.Start(controller.stopCh)
	go sauronInformerFactory.Start(controller.stopCh)

	return controller, nil
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	//create log for run
	logger := zerolog.New(os.Stderr).With().Timestamp().Str("kind", "Controller").Str("name", c.namespace).Logger()

	// Start the informer factories to begin populating the informer caches
	logger.Info().Msg("Starting Sauron controller")

	// Wait for the caches to be synced before starting workers
	logger.Info().Msg("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(c.stopCh, c.clusterRolesSynced, c.configMapsSynced, c.cronJobsSynced,
		c.deploymentsSynced, c.ingressesSynced, c.nodesSynced, c.pvcsSynced, c.roleBindingsSynced, c.secretsSynced,
		c.servicesSynced, c.statefulSetsSynced, c.sauronsSynced, c.storageClassesSynced); !ok {
		return errors.New("failed to wait for caches to sync")
	}

	// register metrics
	metrics.RegisterMetrics()
	// start Metrics server
	go metrics.StartServer(*c.operatorConfig.MetricsPort)

	logger.Info().Msg("Starting workers")
	// Launch two workers to process Sauron resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, c.stopCh)
	}

	logger.Info().Msg("Started workers")
	<-c.stopCh
	logger.Info().Msg("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Sauron resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// Process an update to a Sauron
func (c *Controller) syncHandler(key string) error {
	//create log for syncHandler
	logger := zerolog.New(os.Stderr).With().Timestamp().Str("kind", "Controller").Str("name", c.namespace).Logger()

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return err
	}
	if c.watchVmi != "" && c.watchVmi != name {
		return nil
	}

	// Get the Sauron resource with this namespace/name
	logger.Info().Msgf("[Sauron] Name: %s  NameSpace: %s", name, namespace)
	sauron, err := c.sauronLister.VerrazzanoMonitoringInstances(namespace).Get(name)
	if err != nil {
		runtime.HandleError(fmt.Errorf("error getting Sauron %s in namespace %s: %v", name, namespace, err))
		return err
	}

	return c.syncHandlerStandardMode(sauron)
}

// In Standard Mode, we compare the actual state with the desired, and attempt to
// converge the two.  We then update the Status block of the Sauron resource
// with the current status.
func (c *Controller) syncHandlerStandardMode(sauron *vmcontrollerv1.VerrazzanoMonitoringInstance) error {
	//create log for syncHandler
	logger := zerolog.New(os.Stderr).With().Timestamp().Str("kind", "VerrazzanoMonitoringInstance").Str("name", sauron.Name).Logger()

	originalSauron := sauron.DeepCopy()

	// If lock, controller will not sync/process the Sauron env
	labels := prometheus.Labels{"namespace": sauron.Namespace, "sauron_name": sauron.Name}
	if sauron.Spec.Lock {
		logger.Info().Msgf("[%s/%s] Lock is set to true, this Sauron env will not be synced/processed.", sauron.Name, sauron.Namespace)
		metrics.Lock.With(labels).Set(1)
		return nil
	} else {
		metrics.Lock.Delete(labels)
	}

	/*********************
	 * Initialize Sauron Spec
	 **********************/
	InitializeSauronSpec(c, sauron)

	errorObserved := false

	/*********************
	 * Create RoleBindings
	 **********************/
	err := CreateRoleBindings(c, sauron)
	if err != nil {
		logger.Error().Msgf("Failed to create Role Bindings for sauron: %v", err)
		errorObserved = true
	}

	/*********************
	* Create configmaps
	**********************/
	err = CreateConfigmaps(c, sauron)
	if err != nil {
		logger.Error().Msgf("Failed to create config maps for sauron: %v", err)
		errorObserved = true
	}

	/*********************
	 * Create Services
	 **********************/
	err = CreateServices(c, sauron)
	if err != nil {
		logger.Error().Msgf("Failed to create Services for sauron: %v", err)
		errorObserved = true
	}

	/*********************
	 * Create Persistent Volume Claims
	 **********************/
	pvcToAdMap, err := CreatePersistentVolumeClaims(c, sauron)
	if err != nil {
		logger.Error().Msgf("Failed to create PVCs for sauron: %v", err)
		errorObserved = true
	}

	/*********************
	 * Create Deployments
	 **********************/
	sauronUsername, sauronPassword, err := GetAuthSecrets(c, sauron)
	if err != nil {
		logger.Error().Msgf("Failed to extract Sauron Secrets for sauron: %v", err)
		errorObserved = true
	}
	deploymentsDirty, err := CreateDeployments(c, sauron, pvcToAdMap, sauronUsername, sauronPassword)
	if err != nil {
		logger.Error().Msgf("Failed to create Deployments for sauron: %v", err)
		errorObserved = true
	}
	/*********************
	 * Create StatefulSets
	 **********************/
	err = CreateStatefulSets(c, sauron)
	if err != nil {
		logger.Error().Msgf("Failed to create StatefulSets for sauron: %v", err)
		errorObserved = true
	}

	/*********************
	 * Create Ingresses
	 **********************/
	err = CreateIngresses(c, sauron)
	if err != nil {
		logger.Error().Msgf("Failed to create Ingresses for sauron: %v", err)
		errorObserved = true
	}

	/*********************
	* Update Sauron itself (if necessary, if anything has changed)
	**********************/
	specDiffs := diff.CompareIgnoreTargetEmpties(originalSauron, sauron)
	if specDiffs != "" {
		logger.Debug().Msgf("Acquired lock in namespace: %s", sauron.Namespace)
		logger.Info().Msgf("Sauron %s : Spec differences %s", sauron.Name, specDiffs)
		logger.Info().Msgf("Updating Sauron")
		_, err = c.sauronclientset.VerrazzanoV1().VerrazzanoMonitoringInstances(sauron.Namespace).Update(context.TODO(), sauron, metav1.UpdateOptions{})
		if err != nil {
			logger.Error().Msgf("Failed to update status for sauron: %v", err)
			errorObserved = true
		}
	}
	if !errorObserved && !deploymentsDirty && len(c.buildVersion) > 0 && sauron.Spec.Versioning.CurrentVersion != c.buildVersion {
		// The spec.versioning.currentVersion field should not be updated to the new value until a sync produces no
		// changes.  This allows observers (e.g. the controlled rollout scripts used to put new versions of operator
		// into production) to know when a given sauron has been (mostly) updated, and thus when it's relatively safe to
		// start checking various aspects of the sauron for health.
		sauron.Spec.Versioning.CurrentVersion = c.buildVersion
		_, err = c.sauronclientset.VerrazzanoV1().VerrazzanoMonitoringInstances(sauron.Namespace).Update(context.TODO(), sauron, metav1.UpdateOptions{})
		if err != nil {
			logger.Error().Msgf("Failed to update currentVersion for sauron %s: %v", sauron.Name, err)
		} else {
			logger.Info().Msgf("Updated Sauron %s currentVersion to %s", sauron.Name, c.buildVersion)
		}
	}

	// Create a Hash on sauron/Status object to identify changes to sauron spec
	hash, err := sauron.Hash()
	if err != nil {
		logger.Error().Msgf("Error getting Sauron hash: %v", err)
	}
	if sauron.Status.Hash != hash {
		sauron.Status.Hash = hash
	}

	logger.Info().Msgf("Successfully synced '%s/%s'", sauron.Namespace, sauron.Name)

	return nil
}

// enqueueSauron takes a Sauron resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Sauron.
func (c *Controller) enqueueSauron(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}

	c.workqueue.AddRateLimited(key)
	c.lastEnqueue = time.Now()
}

// IsHealthy returns true if this controller is healthy, false otherwise. It's health is determined based on: (1) its
// workqueue is 0 or decreasing in a timely manner, (2) it can communicate with API server, and (3) the CRD exists.
func (c *Controller) IsHealthy() bool {

	// Make sure if workqueue > 0, make sure it hasn't remained for longer than 60 seconds.
	if startQueueLen := c.workqueue.Len(); startQueueLen > 0 {
		if time.Since(c.lastEnqueue).Seconds() > float64(60) {
			return false
		}
	}

	// Make sure the controller can talk to the API server and its CRD is defined.
	crds, err := c.kubeextclientset.ApiextensionsV1beta1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
	// Error getting CRD from API server
	if err != nil {
		return false
	}
	// No CRDs defined
	if len(crds.Items) == 0 {
		return false
	}
	crdExists := false
	for _, crd := range crds.Items {
		if crd.Name == constants.SauronFullname {
			crdExists = true
		}
	}
	return crdExists
}
