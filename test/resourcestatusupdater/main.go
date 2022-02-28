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

package main

import (
	"context"
	"fmt"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func main() {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()

	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		panic(err.Error())
	}
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	err = mcsv1a1.AddToScheme(scheme.Scheme)
	panicOnError(err)

	dynClient, err := dynamic.NewForConfig(restConfig)
	panicOnError(err)

	restMapper, err := util.BuildRestMapper(restConfig)
	panicOnError(err)

	to, err := resource.ToUnstructured(&mcsv1a1.ServiceExport{})
	panicOnError(err)

	gvr, err := util.FindGroupVersionResource(to, restMapper)
	panicOnError(err)

	ns := "submariner-k8s-broker"
	name := "monti-host-monti-cluster1"

	serviceExportClient := dynClient.Resource(*gvr)
	serviceExport, err := getServiceExport(serviceExportClient, name, ns)
	panicOnError(err)

	fmt.Printf("found %d conditions in ServiceExport\n", len(serviceExport.Status.Conditions))

	now := metav1.Now()
	statusReason := "conflict reason1"
	statusMessage := "conflig msg1"
	exportCondition := mcsv1a1.ServiceExportCondition{
		Type:               mcsv1a1.ServiceExportConflict,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: &now,
		Reason:             &statusReason,
		Message:            &statusMessage,
	}

	serviceExport.Status.Conditions = append(serviceExport.Status.Conditions, exportCondition)
	fmt.Printf("now: %d conditions in ServiceExport\n", len(serviceExport.Status.Conditions))

	raw, err := resource.ToUnstructured(serviceExport)
	panicOnError(err)

	_, err = serviceExportClient.Namespace(ns).UpdateStatus(context.TODO(), raw, metav1.UpdateOptions{})
	panicOnError(err)

	println("done")
}

func getServiceExport(serviceExportClient dynamic.NamespaceableResourceInterface, name, namespace string) (*mcsv1a1.ServiceExport, error) {
	obj, err := serviceExportClient.Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving ServiceExport")
	}

	se := &mcsv1a1.ServiceExport{}

	err = scheme.Scheme.Convert(obj, se, nil)
	if err != nil {
		return nil, errors.WithMessagef(err, "Error converting %#v to ServiceExport", obj)
	}

	return se, nil
}

func panicOnError(err error) {
	if err != nil {
		panic(err.Error())
	}
}
