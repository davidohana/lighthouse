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
	"reflect"
	"strings"

	"github.com/pkg/errors"
	lhconstants "github.com/submariner-io/lighthouse/pkg/constants"
	discovery "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var (
	serviceExportGVR = schema.GroupVersionResource{
		Group:    mcsv1a1.GroupName,
		Version:  mcsv1a1.GroupVersion.Version,
		Resource: getPluralTypeName(mcsv1a1.ServiceExport{}),
	}

	serviceImportGVR = schema.GroupVersionResource{
		Group:    mcsv1a1.GroupName,
		Version:  mcsv1a1.GroupVersion.Version,
		Resource: getPluralTypeName(mcsv1a1.ServiceImport{}),
	}

	endpointSliceGVR = schema.GroupVersionResource{
		Group:    discovery.GroupName,
		Version:  discovery.SchemeGroupVersion.Version,
		Resource: getPluralTypeName(discovery.EndpointSlice{}),
	}
)

func getPluralTypeName(obj interface{}) string {
	return strings.ToLower(reflect.TypeOf(obj).Name()) + "s"
}

func (a *Controller) Cleanup() error {
	notInBrokerNamespace := fields.OneTermNotEqualSelector("metadata.namespace", a.brokerNamespace).String()
	listOptionsFilterByLHSourceClusterLabel := metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{lhconstants.LighthouseLabelSourceCluster: a.clusterID}).String(),
	}
	delOptions := metav1.DeleteOptions{}

	logger.Info("Deleting all ServiceImports from the local cluster")
	//  skipping those in the broker namespace (if the broker is on the local cluster)
	err := a.localClient.Resource(serviceImportGVR).Namespace(metav1.NamespaceAll).DeleteCollection(
		context.TODO(), delOptions,
		metav1.ListOptions{FieldSelector: notInBrokerNamespace})
	if err != nil {
		return errors.Wrap(err, "error deleting local ServiceImports")
	}

	logger.Info("Deleting all ServiceExports originated from this cluster from the broker")

	err = a.brokerClient.Resource(serviceExportGVR).Namespace(a.brokerNamespace).DeleteCollection(
		context.TODO(), delOptions, listOptionsFilterByLHSourceClusterLabel)
	if err != nil {
		return errors.Wrap(err, "error deleting remote ServiceExports")
	}

	logger.Info("Deleting all EndpointSlices from the local cluster")

	// skipping those in the broker namespace (if the broker is on the local cluster).
	err = a.localClient.Resource(endpointSliceGVR).Namespace(metav1.NamespaceAll).DeleteCollection(
		context.TODO(), delOptions,
		metav1.ListOptions{
			FieldSelector: notInBrokerNamespace,
			LabelSelector: labels.Set(map[string]string{discovery.LabelManagedBy: lhconstants.LabelValueManagedBy}).String(),
		})
	if err != nil {
		return errors.Wrap(err, "error deleting local EndpointSlices")
	}

	logger.Info("Deleting all EndpointSlices originated from this cluster from the broker")

	err = a.brokerClient.Resource(endpointSliceGVR).Namespace(a.brokerNamespace).DeleteCollection(
		context.TODO(), delOptions,
		metav1.ListOptions{
			LabelSelector: labels.Set(map[string]string{lhconstants.MCSLabelSourceCluster: a.clusterID}).String(),
		})
	if err != nil {
		return errors.Wrap(err, "error deleting remote EndpointSlices")
	}

	return nil
}
