/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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
package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/submariner-io/admiral/pkg/log"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/syncer"
	"github.com/submariner-io/admiral/pkg/syncer/broker"
	"github.com/submariner-io/admiral/pkg/util"
	lhconstants "github.com/submariner-io/lighthouse/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

const (
	reasonServiceUnavailable = "ServiceUnavailable"
	reasonInvalidServiceType = "UnsupportedServiceType"
	clusterIP                = "cluster-ip"
)

type AgentConfig struct {
	ServiceImportCounterName                string
	ServiceExportCounterName                string
	ServiceExportUploadsCounterName         string
	ServiceExportStatusDownloadsCounterName string
}

var MaxExportStatusConditions = 10

// nolint:gocritic // (hugeParam) This function modifies syncerConf so we don't want to pass by pointer.
func New(spec *AgentSpecification, syncerConf broker.SyncerConfig, kubeClientSet kubernetes.Interface,
	syncerMetricNames AgentConfig) (*Controller, error) {
	agentController := &Controller{
		clusterID:        spec.ClusterID,
		namespace:        spec.Namespace,
		globalnetEnabled: spec.GlobalnetEnabled,
		kubeClientSet:    kubeClientSet,
	}

	_, gvr, err := util.ToUnstructuredResource(&mcsv1a1.ServiceExport{}, syncerConf.RestMapper)
	if err != nil {
		return nil, errors.Wrap(err, "error converting resource")
	}

	agentController.serviceExportClient = syncerConf.LocalClient.Resource(*gvr)

	syncerConf.LocalNamespace = spec.Namespace
	syncerConf.LocalClusterID = spec.ClusterID

	syncerConf.ResourceConfigs = []broker.ResourceConfig{
		{
			LocalSourceNamespace: metav1.NamespaceAll,
			LocalResourceType:    &mcsv1a1.ServiceImport{},
			BrokerResourceType:   &mcsv1a1.ServiceImport{},
			SyncCounterOpts: &prometheus.GaugeOpts{
				Name: syncerMetricNames.ServiceImportCounterName,
				Help: "Count of imported services",
			},
		},
	}

	agentController.serviceImportSyncer, err = broker.NewSyncer(syncerConf)
	if err != nil {
		return nil, errors.Wrap(err, "error creating ServiceImport syncer")
	}

	syncerConf.LocalNamespace = metav1.NamespaceAll
	syncerConf.ResourceConfigs = []broker.ResourceConfig{
		{
			LocalSourceNamespace: metav1.NamespaceAll,
			LocalResourceType:    &discovery.EndpointSlice{},
			LocalShouldProcess:   agentController.endpointSliceSyncerSyncerShouldProcessResource,
			//LocalTransform:       agentController.filterLocalEndpointSlices,
			LocalResourcesEquivalent: func(obj1, obj2 *unstructured.Unstructured) bool {
				return false
			},
			BrokerResourceType: &discovery.EndpointSlice{},
			BrokerResourcesEquivalent: func(obj1, obj2 *unstructured.Unstructured) bool {
				return false
			},
			BrokerTransform: agentController.remoteEndpointSliceToLocal,
		},
	}

	agentController.endpointSliceSyncer, err = broker.NewSyncer(syncerConf)
	if err != nil {
		return nil, errors.Wrap(err, "error creating EndpointSlice syncer")
	}

	//agentController.serviceExportSyncer, err = syncer.NewResourceSyncer(&syncer.ResourceSyncerConfig{
	//	Name:             "ServiceExport -> ServiceImport",
	//	SourceClient:     syncerConf.LocalClient,
	//	SourceNamespace:  metav1.NamespaceAll,
	//	RestMapper:       syncerConf.RestMapper,
	//	Federator:        agentController.serviceImportSyncer.GetLocalFederator(),
	//	ResourceType:     &mcsv1a1.ServiceExport{},
	//	Transform:        agentController.serviceExportToServiceImport,
	//	OnSuccessfulSync: agentController.onSuccessfulServiceImportSync,
	//	Scheme:           syncerConf.Scheme,
	//	SyncCounterOpts: &prometheus.GaugeOpts{
	//		Name: syncerMetricNames.ServiceExportCounterName,
	//		Help: "Count of exported services",
	//	},
	//})
	//if err != nil {
	//	return nil, errors.Wrap(err, "error creating ServiceExport syncer")
	//}

	// This syncer will:
	// - Upload local service exports to the broker (only if service exist)
	// - Add labels and annotations to preserve information out of schema
	// - Update the service export on the broker when there is a local update
	// - Delete the service export on the broker when local service export is deleted
	agentController.serviceExportUploader, err = syncer.NewResourceSyncer(&syncer.ResourceSyncerConfig{
		Name:             "ServiceExport Uploader",
		LocalClusterID:   spec.ClusterID,
		SourceClient:     syncerConf.LocalClient,
		SourceNamespace:  metav1.NamespaceAll,
		RestMapper:       syncerConf.RestMapper,
		Federator:        agentController.serviceImportSyncer.GetBrokerFederator(),
		Direction:        syncer.LocalToRemote,
		ResourceType:     &mcsv1a1.ServiceExport{},
		Transform:        agentController.serviceExportUploadTransform,
		OnSuccessfulSync: agentController.onSuccessfulServiceExportSync,
		Scheme:           syncerConf.Scheme,
		SyncCounterOpts: &prometheus.GaugeOpts{
			Name: syncerMetricNames.ServiceExportUploadsCounterName,
			Help: "Count of uploaded service exports",
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating ServiceExport uploader")
	}

	// this syncer downloads conflict condition status of service exports from the broker and merge it
	// into the local service export
	agentController.serviceExportStatusDownloader, err = syncer.NewResourceSyncer(&syncer.ResourceSyncerConfig{
		Name:            "ServiceExport.Status Downloader",
		LocalClusterID:  spec.ClusterID,
		SourceClient:    agentController.endpointSliceSyncer.GetBrokerClient(),
		SourceNamespace: agentController.endpointSliceSyncer.GetBrokerNamespace(),
		RestMapper:      syncerConf.RestMapper,
		Federator:       agentController.serviceImportSyncer.GetLocalFederator(),
		Direction:       syncer.None, // handle filtering of exports manually in transform func
		ResourceType:    &mcsv1a1.ServiceExport{},
		Transform:       agentController.serviceExportDownloadTransform,
		Scheme:          syncerConf.Scheme,
		SyncCounterOpts: &prometheus.GaugeOpts{
			Name: syncerMetricNames.ServiceExportStatusDownloadsCounterName,
			Help: "Count of exported services",
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating ServiceExport Status Downloader")
	}

	// this syncer will delete service exports at the broker when the correlated local service is deleted
	agentController.serviceSyncer, err = syncer.NewResourceSyncer(&syncer.ResourceSyncerConfig{
		Name:            "Service deletion watcher",
		SourceClient:    syncerConf.LocalClient,
		SourceNamespace: metav1.NamespaceAll,
		RestMapper:      syncerConf.RestMapper,
		Federator:       agentController.serviceImportSyncer.GetBrokerFederator(),
		ResourceType:    &corev1.Service{},
		ShouldProcess:   agentController.serviceSyncerShouldProcessResource,
		Transform:       agentController.serviceToRemoteServiceExport,
		Scheme:          syncerConf.Scheme,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating Service syncer")
	}

	agentController.serviceImportController, err = newServiceImportController(spec, agentController.serviceSyncer,
		syncerConf.RestMapper, syncerConf.LocalClient, syncerConf.Scheme)
	if err != nil {
		return nil, err
	}

	return agentController, nil
}

func (a *Controller) Start(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Agent controller")

	//if err := a.serviceExportSyncer.Start(stopCh); err != nil {
	//	return errors.Wrap(err, "error starting ServiceExport syncer")
	//}

	if err := a.serviceExportUploader.Start(stopCh); err != nil {
		return errors.Wrap(err, "error starting ServiceExport uploader")
	}

	if err := a.serviceExportStatusDownloader.Start(stopCh); err != nil {
		return errors.Wrap(err, "error starting ServiceExport status downloader")
	}

	if err := a.serviceSyncer.Start(stopCh); err != nil {
		return errors.Wrap(err, "error starting Service syncer")
	}

	if err := a.endpointSliceSyncer.Start(stopCh); err != nil {
		return errors.Wrap(err, "error starting EndpointSlice syncer")
	}

	if err := a.serviceImportSyncer.Start(stopCh); err != nil {
		return errors.Wrap(err, "error starting ServiceImport syncer")
	}

	if err := a.serviceImportController.start(stopCh); err != nil {
		return errors.Wrap(err, "error starting ServiceImport controller")
	}

	// check on startup if local service exports still exist for the local service imports.
	// enqueue deletion of export if not, to delete obsolete service import
	//a.serviceExportSyncer.Reconcile(func() []runtime.Object {
	//	return a.serviceImportLister(func(si *mcsv1a1.ServiceImport) runtime.Object {
	//		return &mcsv1a1.ServiceExport{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name:      si.GetAnnotations()[lhconstants.OriginName],
	//				Namespace: si.GetAnnotations()[lhconstants.OriginNamespace],
	//			},
	//		}
	//	})
	//})

	// check on startup if a local service export still exist for all remote service exports uploaded from this cluster.
	// if not - enqueue deletion of the service export, to delete obsolete service export from the broker
	a.serviceExportUploader.Reconcile(func() []runtime.Object {
		return a.remoteServiceExportLister(func(se *mcsv1a1.ServiceExport) runtime.Object {
			annotations := se.GetAnnotations()
			// workaround a bug in resource_syncer.Reconcile() when in LocalToRemote mode:
			// resource name on broker is not mapped back to resource name on origin cluster
			se.Name = annotations[lhconstants.OriginName]
			return se
		})
	})

	// check on startup if a local service still exist for all remote service exports uploaded from this cluster.
	// if not - enqueue deletion of the service, to delete obsolete service export from the broker
	a.serviceSyncer.Reconcile(func() []runtime.Object {
		return a.remoteServiceExportLister(func(se *mcsv1a1.ServiceExport) runtime.Object {
			// only care about service exports that originated from this cluster
			if !a.isRemoteServiceExportOwned(se) {
				return nil
			}
			annotations := se.GetAnnotations()
			return &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      annotations[lhconstants.OriginName],
					Namespace: annotations[lhconstants.OriginNamespace],
				},
			}
		})
	})

	klog.Info("Agent controller started")

	return nil
}

func (a *Controller) remoteServiceExportLister(transform func(si *mcsv1a1.ServiceExport) runtime.Object) []runtime.Object {
	brokerSeList, err := a.serviceExportStatusDownloader.ListResources()

	if err != nil {
		klog.Errorf("Error listing broker's ServiceExports: %v", err)
		return nil
	}

	retList := make([]runtime.Object, 0, len(brokerSeList))

	for _, obj := range brokerSeList {
		se := obj.(*mcsv1a1.ServiceExport)

		var transformed runtime.Object
		if transform == nil {
			transformed = se
		} else {
			transformed = transform(se)
		}
		if transformed != nil {
			retList = append(retList, transformed)
		}
	}

	return retList
}

func (a *Controller) serviceImportLister(transform func(si *mcsv1a1.ServiceImport) runtime.Object) []runtime.Object {
	siList, err := a.serviceImportSyncer.ListLocalResources(&mcsv1a1.ServiceImport{})
	if err != nil {
		klog.Errorf("Error listing serviceImports: %v", err)
		return nil
	}

	retList := make([]runtime.Object, 0, len(siList))

	for _, obj := range siList {
		si := obj.(*mcsv1a1.ServiceImport)

		retList = append(retList, transform(si))
	}

	return retList
}

func (a *Controller) serviceExportToServiceImport(obj runtime.Object, _ int, op syncer.Operation) (runtime.Object, bool) {
	svcExport := obj.(*mcsv1a1.ServiceExport)

	klog.V(log.DEBUG).Infof("ServiceExport %s/%s %sd", svcExport.Namespace, svcExport.Name, op)

	if op == syncer.Delete {
		return a.newServiceImport(svcExport.Name, svcExport.Namespace), false
	}

	obj, found, err := a.serviceSyncer.GetResource(svcExport.Name, svcExport.Namespace)
	if err != nil {
		// some other error. Log and requeue
		a.updateExportedServiceStatus(svcExport.Name, svcExport.Namespace,
			mcsv1a1.ServiceExportValid, corev1.ConditionUnknown, "ServiceRetrievalFailed",
			fmt.Sprintf("Error retrieving the Service: %v", err))
		klog.Errorf("Error retrieving Service (%s/%s): %v", svcExport.Namespace, svcExport.Name, err)

		return nil, true
	}

	if !found {
		klog.V(log.DEBUG).Infof("Service to be exported (%s/%s) doesn't exist", svcExport.Namespace, svcExport.Name)
		a.updateExportedServiceStatus(svcExport.Name, svcExport.Namespace,
			mcsv1a1.ServiceExportValid, corev1.ConditionFalse, reasonServiceUnavailable,
			"Service to be exported doesn't exist")

		return nil, true
	}

	if op == syncer.Update && getLastExportConditionReason(svcExport) != reasonServiceUnavailable {
		return nil, false
	}

	svc := obj.(*corev1.Service)

	svcType, ok := getServiceImportType(svc)

	if !ok {
		a.updateExportedServiceStatus(svcExport.Name, svcExport.Namespace,
			mcsv1a1.ServiceExportValid, corev1.ConditionFalse, reasonInvalidServiceType,
			fmt.Sprintf("Service of type %v not supported", svc.Spec.Type))
		klog.Errorf("Service type %q not supported", svc.Spec.Type)

		return nil, false
	}

	serviceImport := a.newServiceImport(svcExport.Name, svcExport.Namespace)

	serviceImport.Spec = mcsv1a1.ServiceImportSpec{
		Ports:                 []mcsv1a1.ServicePort{},
		Type:                  svcType,
		SessionAffinityConfig: new(corev1.SessionAffinityConfig),
	}

	serviceImport.Status = mcsv1a1.ServiceImportStatus{
		Clusters: []mcsv1a1.ClusterStatus{
			{
				Cluster: a.clusterID,
			},
		},
	}

	if svcType == mcsv1a1.ClusterSetIP {
		if a.globalnetEnabled {
			ip, reason, msg := a.getGlobalIP(svc)
			if ip == "" {
				klog.V(log.DEBUG).Infof("Service to be exported (%s/%s) doesn't have a global IP yet", svcExport.Namespace, svcExport.Name)
				// Globalnet enabled but service doesn't have globalIp yet, Update the status and requeue
				a.updateExportedServiceStatus(svcExport.Name, svcExport.Namespace,
					mcsv1a1.ServiceExportValid, corev1.ConditionFalse, reason, msg)

				return nil, true
			}

			serviceImport.Spec.IPs = []string{ip}
		} else {
			serviceImport.Spec.IPs = []string{svc.Spec.ClusterIP}
		}

		serviceImport.Spec.Ports = a.getPortsForService(svc)
		/* We also store the clusterIP in an annotation as an optimization to recover it in case the IPs are
		cleared out when here's no backing Endpoint pods.
		*/
		serviceImport.Annotations[clusterIP] = serviceImport.Spec.IPs[0]
	}

	a.updateExportedServiceStatus(svcExport.Name, svcExport.Namespace,
		mcsv1a1.ServiceExportValid, corev1.ConditionFalse, "AwaitingSync",
		"Awaiting sync of the ServiceImport to the broker")

	klog.V(log.DEBUG).Infof("Returning ServiceImport: %#v", serviceImport)

	return serviceImport, false
}

func (a *Controller) serviceExportUploadTransform(serviceObj runtime.Object, _ int, op syncer.Operation) (runtime.Object, bool) {
	localServiceExport := serviceObj.(*mcsv1a1.ServiceExport)

	klog.V(log.DEBUG).Infof("Local ServiceExport %s/%s %sd", localServiceExport.Namespace, localServiceExport.Name, op)

	brokerServiceExport := a.newServiceExport(localServiceExport.Name, localServiceExport.Namespace)
	if op == syncer.Delete {
		// need to convert name to full name so that resource_syncer can find the resource
		// on the broker and delete it
		return brokerServiceExport, false
	}

	serviceObj, serviceExist, err := a.serviceSyncer.GetResource(localServiceExport.Name, localServiceExport.Namespace)
	if err != nil {
		// some other error. Log and requeue to retry later
		a.updateExportedServiceStatus(localServiceExport.Name, localServiceExport.Namespace,
			mcsv1a1.ServiceExportValid, corev1.ConditionUnknown, "ServiceRetrievalFailed",
			fmt.Sprintf("Error retrieving the Service: %v", err))
		klog.Errorf("Error retrieving Service (%s/%s): %v", localServiceExport.Namespace, localServiceExport.Name, err)

		return nil, true
	}

	if !serviceExist {
		// corresponding service not exist - requeue to retry later, until service is created
		klog.V(log.DEBUG).Infof("Service to be exported (%s/%s) doesn't exist, will not upload",
			localServiceExport.Namespace, localServiceExport.Name)
		a.updateExportedServiceStatus(localServiceExport.Name, localServiceExport.Namespace,
			mcsv1a1.ServiceExportValid, corev1.ConditionFalse, reasonServiceUnavailable,
			"Service to be exported doesn't exist")

		return nil, true
	}

	// this is an update and service exist. we want to continue only if the last status is "service unavailable"
	// which means that service is available again -> the export should be uploaded again to the broker
	if op == syncer.Update && getLastExportConditionReason(localServiceExport) != reasonServiceUnavailable {
		return nil, false
	}

	//svc := serviceObj.(*corev1.Service)

	//svcType, ok := getServiceImportType(svc)

	//if !ok {
	//	a.updateExportedServiceStatus(localServiceExport.Name, localServiceExport.Namespace, corev1.ConditionFalse, reasonInvalidServiceType,
	//		fmt.Sprintf("Service of type %v not supported", svc.Spec.Type))
	//	klog.Errorf("Service type %q not supported", svc.Spec.Type)
	//
	//	return nil, false
	//}

	//serviceImport := a.newServiceImport(localServiceExport.Name, localServiceExport.Namespace)

	//serviceImport.Spec = mcsv1a1.ServiceImportSpec{
	//	Ports:                 []mcsv1a1.ServicePort{},
	//	Type:                  svcType,
	//	SessionAffinityConfig: new(corev1.SessionAffinityConfig),
	//}
	//
	//serviceImport.Status = mcsv1a1.ServiceImportStatus{
	//	Clusters: []mcsv1a1.ClusterStatus{
	//		{
	//			Cluster: a.clusterID,
	//		},
	//	},
	//}

	//if svcType == mcsv1a1.ClusterSetIP {
	//	if a.globalnetEnabled {
	//		ip, reason, msg := a.getGlobalIP(svc)
	//		if ip == "" {
	//			klog.V(log.DEBUG).Infof("Service to be exported (%s/%s) doesn't have a global IP yet", localServiceExport.Namespace, localServiceExport.Name)
	//			// Globalnet enabled but serviceObj doesn't have globalIp yet, Update the status and requeue
	//			a.updateExportedServiceStatus(localServiceExport.Name, localServiceExport.Namespace, corev1.ConditionFalse, reason, msg)
	//
	//			return nil, true
	//		}
	//
	//		serviceImport.Spec.IPs = []string{ip}
	//	} else {
	//		serviceImport.Spec.IPs = []string{svc.Spec.ClusterIP}
	//	}
	//
	//	serviceImport.Spec.Ports = a.getPortsForService(svc)
	//	/* We also store the clusterIP in an annotation as an optimization to recover it in case the IPs are
	//	cleared out when here's no backing Endpoint pods.
	//	*/
	//	serviceImport.Annotations[clusterIP] = serviceImport.Spec.IPs[0]
	//}

	a.updateExportedServiceStatus(localServiceExport.Name, localServiceExport.Namespace,
		mcsv1a1.ServiceExportValid, corev1.ConditionFalse, "AwaitingSync",
		"Awaiting sync of the ServiceExport to the broker")

	klog.V(log.DEBUG).Infof("Returning ServiceExport: %#v", brokerServiceExport)

	return brokerServiceExport, false
}

// check whether a broker's service export originates from the local cluster
func (a *Controller) isRemoteServiceExportOwned(se *mcsv1a1.ServiceExport) bool {
	return se.GetLabels()[lhconstants.LighthouseLabelSourceCluster] == a.clusterID
}

func (a *Controller) serviceExportDownloadTransform(obj runtime.Object, _ int, op syncer.Operation) (runtime.Object, bool) {
	if op != syncer.Update {
		// we only care about status updates
		return nil, false
	}

	brokerServiceExport := obj.(*mcsv1a1.ServiceExport)

	// we only care about service exports originating from the local cluster
	if !a.isRemoteServiceExportOwned(brokerServiceExport) {
		return nil, false
	}

	klog.V(log.DEBUG).Infof("ServiceExport %s/%s on broker %sd",
		brokerServiceExport.Namespace, brokerServiceExport.Name, op)

	conflictCondition := getServiceExportCondition(&brokerServiceExport.Status, mcsv1a1.ServiceExportConflict)
	if conflictCondition == nil {
		// no conflict condition, do nothing
		// assuming even if there was a conflict and now resolved, there will be conflict condition
		// with a status of false
		return nil, false
	}

	annotations := brokerServiceExport.GetAnnotations()
	originName := annotations[lhconstants.OriginName]
	originNamespace := annotations[lhconstants.OriginNamespace]

	// update conflict status in local service export to reflect broker's state.
	// use own clock instead of broker's clock for time consistency with other status updates
	a.updateExportedServiceStatus(originName, originNamespace,
		conflictCondition.Type, conflictCondition.Status, *conflictCondition.Reason, *conflictCondition.Message)

	return nil, false
}

func getServiceExportCondition(status *mcsv1a1.ServiceExportStatus, conditionType mcsv1a1.ServiceExportConditionType) *mcsv1a1.ServiceExportCondition {
	for i := range status.Conditions {
		// iterate in reverse to get the last condition of the requested type
		// (assuming new conditions are appended at the end of slice)
		c := status.Conditions[len(status.Conditions)-1-i]
		if c.Type == conditionType {
			return &c
		}
	}
	return nil
}

func getLastExportConditionReason(svcExport *mcsv1a1.ServiceExport) string {
	numCond := len(svcExport.Status.Conditions)
	if numCond > 0 && svcExport.Status.Conditions[numCond-1].Reason != nil {
		return *svcExport.Status.Conditions[numCond-1].Reason
	}

	return ""
}

func getServiceImportType(service *corev1.Service) (mcsv1a1.ServiceImportType, bool) {
	if service.Spec.Type != "" && service.Spec.Type != corev1.ServiceTypeClusterIP {
		return "", false
	}

	if service.Spec.ClusterIP == corev1.ClusterIPNone {
		return mcsv1a1.Headless, true
	}

	return mcsv1a1.ClusterSetIP, true
}

func (a *Controller) onSuccessfulServiceImportSync(synced runtime.Object, op syncer.Operation) {
	if op == syncer.Delete {
		return
	}

	serviceImport := synced.(*mcsv1a1.ServiceImport)

	annotations := serviceImport.GetAnnotations()
	a.updateExportedServiceStatus(annotations[lhconstants.OriginName], annotations[lhconstants.OriginNamespace],
		mcsv1a1.ServiceExportValid, corev1.ConditionTrue, "",
		"Service was successfully synced to the broker")
}

func (a *Controller) onSuccessfulServiceExportSync(synced runtime.Object, op syncer.Operation) {
	if op == syncer.Delete {
		return
	}

	serviceExport := synced.(*mcsv1a1.ServiceExport)

	annotations := serviceExport.GetAnnotations()
	a.updateExportedServiceStatus(
		annotations[lhconstants.OriginName],
		annotations[lhconstants.OriginNamespace],
		mcsv1a1.ServiceExportValid, corev1.ConditionTrue, "",
		"ServiceExport was successfully synced to the broker")
}

func (a *Controller) serviceSyncerShouldProcessResource(_ *unstructured.Unstructured, op syncer.Operation) bool {
	// we only care about service deletion so that we are able to delete corresponding exports from the broker
	// actually functionality will be the same without this function because there is a subsequenct check at the
	// transform function, but it prevents unnecessary verbose logging
	return op == syncer.Delete
}

func (a *Controller) serviceToRemoteServiceExport(obj runtime.Object, _ int, op syncer.Operation) (runtime.Object, bool) {
	if op != syncer.Delete {
		// Ignore create/update
		return nil, false
	}

	svc := obj.(*corev1.Service)
	klog.V(log.DEBUG).Infof("Service deleted: %s/%s", svc.Name, svc.Namespace)

	obj, found, err := a.serviceExportUploader.GetResource(svc.Name, svc.Namespace)
	if err != nil {
		// some other error. Log and requeue
		klog.Errorf("Error retrieving ServiceExport for Service (%s/%s): %v", svc.Namespace, svc.Name, err)
		return nil, true
	}

	if !found {
		klog.V(log.DEBUG).Infof("ServiceExport not found for deleted service: %s/%s", svc.Namespace, svc.Name)
		return nil, false
	}

	localServiceExport := obj.(*mcsv1a1.ServiceExport)

	klog.V(log.DEBUG).Infof("ServiceExport found for deleted service: %s/%s, will delete broker's copy",
		localServiceExport.Name, localServiceExport.Namespace)

	// rename the service export to expected name on broker so that federator can find and delete it
	brokerServiceExport := a.newServiceExport(localServiceExport.Name, localServiceExport.Namespace)

	// Update the local export status and requeue
	// this will make service export upload syncer to retry syncing until service is re-created
	a.updateExportedServiceStatus(localServiceExport.Name, localServiceExport.Namespace,
		mcsv1a1.ServiceExportValid, corev1.ConditionFalse, reasonServiceUnavailable, "Service to be exported was deleted")

	return brokerServiceExport, false
}

func (a *Controller) updateExportedServiceStatus(name, namespace string,
	conditionType mcsv1a1.ServiceExportConditionType, status corev1.ConditionStatus, reason, msg string) {

	klog.V(log.DEBUG).Infof("updateExportedServiceStatus for (%s/%s) - Type: %q, Status: %q, Reason: %q, Message: %q",
		namespace, name, conditionType, status, reason, msg)

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		toUpdate, err := a.getServiceExport(name, namespace)
		if apierrors.IsNotFound(err) {
			klog.Infof("ServiceExport (%s/%s) not found - unable to update status", namespace, name)
			return nil
		} else if err != nil {
			return err
		}

		now := metav1.Now()
		exportCondition := mcsv1a1.ServiceExportCondition{
			Type:               conditionType,
			Status:             status,
			LastTransitionTime: &now,
			Reason:             &reason,
			Message:            &msg,
		}

		numCond := len(toUpdate.Status.Conditions)
		if numCond > 0 && serviceExportConditionEqual(&toUpdate.Status.Conditions[numCond-1], &exportCondition) {
			klog.V(log.TRACE).Infof("Last ServiceExportCondition for (%s/%s) is equal - not updating status: %#v",
				namespace, name, toUpdate.Status.Conditions[numCond-1])
			return nil
		}

		if numCond >= MaxExportStatusConditions {
			copy(toUpdate.Status.Conditions[0:], toUpdate.Status.Conditions[1:])
			toUpdate.Status.Conditions = toUpdate.Status.Conditions[:MaxExportStatusConditions]
			toUpdate.Status.Conditions[MaxExportStatusConditions-1] = exportCondition
		} else {
			toUpdate.Status.Conditions = append(toUpdate.Status.Conditions, exportCondition)
		}

		raw, err := resource.ToUnstructured(toUpdate)
		if err != nil {
			return errors.Wrap(err, "error converting resource")
		}

		_, err = a.serviceExportClient.Namespace(toUpdate.Namespace).UpdateStatus(context.TODO(), raw, metav1.UpdateOptions{})

		return errors.Wrap(err, "error from UpdateStatus")
	})

	if retryErr != nil {
		klog.Errorf("Error updating status for ServiceExport (%s/%s): %+v", namespace, name, retryErr)
	}
}

func (a *Controller) getServiceExport(name, namespace string) (*mcsv1a1.ServiceExport, error) {
	obj, err := a.serviceExportClient.Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving ServiceExport")
	}

	se := &mcsv1a1.ServiceExport{}

	err = a.serviceImportController.scheme.Convert(obj, se, nil)
	if err != nil {
		return nil, errors.WithMessagef(err, "Error converting %#v to ServiceExport", obj)
	}

	return se, nil
}

func serviceExportConditionEqual(c1, c2 *mcsv1a1.ServiceExportCondition) bool {
	return c1.Type == c2.Type && c1.Status == c2.Status && reflect.DeepEqual(c1.Reason, c2.Reason) &&
		reflect.DeepEqual(c1.Message, c2.Message)
}

func (a *Controller) newServiceExport(name, namespace string) *mcsv1a1.ServiceExport {
	return &mcsv1a1.ServiceExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.getObjectNameWithClusterID(name, namespace),
			Namespace: namespace,
			Annotations: map[string]string{
				lhconstants.OriginName:      name,
				lhconstants.OriginNamespace: namespace,
			},
			Labels: map[string]string{
				lhconstants.LighthouseLabelSourceName:    name,
				lhconstants.LabelSourceNamespace:         namespace,
				lhconstants.LighthouseLabelSourceCluster: a.clusterID,
			},
		},
	}
}

func (a *Controller) newServiceImport(name, namespace string) *mcsv1a1.ServiceImport {
	return &mcsv1a1.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{
			Name: a.getObjectNameWithClusterID(name, namespace),
			Annotations: map[string]string{
				lhconstants.OriginName:      name,
				lhconstants.OriginNamespace: namespace,
			},
			Labels: map[string]string{
				lhconstants.LighthouseLabelSourceName:    name,
				lhconstants.LabelSourceNamespace:         namespace,
				lhconstants.LighthouseLabelSourceCluster: a.clusterID,
			},
		},
	}
}

func (a *Controller) getPortsForService(service *corev1.Service) []mcsv1a1.ServicePort {
	mcsPorts := make([]mcsv1a1.ServicePort, 0, len(service.Spec.Ports))

	for _, port := range service.Spec.Ports {
		mcsPorts = append(mcsPorts, mcsv1a1.ServicePort{
			Name:     port.Name,
			Protocol: port.Protocol,
			Port:     port.Port,
		})
	}

	return mcsPorts
}

func (a *Controller) getObjectNameWithClusterID(name, namespace string) string {
	return name + "-" + namespace + "-" + a.clusterID
}

func (a *Controller) remoteEndpointSliceToLocal(obj runtime.Object, _ int, _ syncer.Operation) (runtime.Object, bool) {
	endpointSlice := obj.(*discovery.EndpointSlice)
	endpointSlice.Namespace = endpointSlice.GetObjectMeta().GetLabels()[lhconstants.LabelSourceNamespace]

	return endpointSlice, false
}

func (a *Controller) endpointSliceSyncerSyncerShouldProcessResource(obj *unstructured.Unstructured, _ syncer.Operation) bool {
	// we only want to sync local endpoint slices managed by submariner
	// filter here and not in transform() prevents unnecessary verbose logs
	labels := obj.GetLabels()
	return labels[discovery.LabelManagedBy] == lhconstants.LabelValueManagedBy
}

//func (a *Controller) filterLocalEndpointSlices(obj runtime.Object, _ int, _ syncer.Operation) (runtime.Object, bool) {
//	endpointSlice := obj.(*discovery.EndpointSlice)
//	labels := endpointSlice.GetObjectMeta().GetLabels()
//
//	if labels[discovery.LabelManagedBy] != lhconstants.LabelValueManagedBy {
//		return nil, false
//	}
//
//	return obj, false
//}

func (a *Controller) getGlobalIP(service *corev1.Service) (ip, reason, msg string) {
	if a.globalnetEnabled {
		ingressIP, found := a.getIngressIP(service.Name, service.Namespace)
		if !found {
			return "", defaultReasonIPUnavailable, defaultMsgIPUnavailable
		}

		return ingressIP.allocatedIP, ingressIP.unallocatedReason, ingressIP.unallocatedMsg
	}

	return "", "GlobalnetDisabled", "Globalnet is not enabled"
}

func (a *Controller) getIngressIP(name, namespace string) (*IngressIP, bool) {
	obj, found := a.serviceImportController.globalIngressIPCache.getForService(namespace, name)
	if !found {
		return nil, false
	}

	return parseIngressIP(obj), true
}
