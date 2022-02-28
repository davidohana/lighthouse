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
	"flag"
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
	condType := string(mcsv1a1.ServiceExportConflict)
	condReason := "default reason"
	condMsg := "deafult message"
	condStatus := string(corev1.ConditionTrue)

	seNamespace := "submariner-k8s-broker"
	seName := "svc1-ns1-cluster1"
	help := false

	flagset := flag.CommandLine
	flagset.BoolVar(&help, "help", help, "show help")
	flagset.StringVar(&condType, "type", condType, "condition type")
	flagset.StringVar(&condReason, "reason", condReason, "condition reason")
	flagset.StringVar(&condMsg, "message", condMsg, "condition message")
	flagset.StringVar(&condStatus, "status", condStatus, "condition status")
	flagset.StringVar(&seNamespace, "namespace", seNamespace, "namespace of service export")
	flagset.StringVar(&seName, "name", seName, "name of service export")
	flag.Parse()
	if help {
		flag.PrintDefaults()
		return
	}

	flag.VisitAll(func(f *flag.Flag) {
		fmt.Printf("arg: %s: %s\n", f.Name, f.Value)
	})

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	panicOnError(err)

	clientset, err := kubernetes.NewForConfig(restConfig)
	panicOnError(err)

	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	panicOnError(err)
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

	serviceExportClient := dynClient.Resource(*gvr)
	serviceExport, err := getServiceExport(serviceExportClient, seName, seNamespace)
	panicOnError(err)

	fmt.Printf("found %d conditions in ServiceExport\n", len(serviceExport.Status.Conditions))

	now := metav1.Now()
	exportCondition := mcsv1a1.ServiceExportCondition{
		LastTransitionTime: &now,
		Type:               mcsv1a1.ServiceExportConditionType(condType),
		Status:             corev1.ConditionStatus(condStatus),
		Reason:             &condReason,
		Message:            &condMsg,
	}

	serviceExport.Status.Conditions = append(serviceExport.Status.Conditions, exportCondition)
	fmt.Printf("now: %d conditions in ServiceExport\n", len(serviceExport.Status.Conditions))

	raw, err := resource.ToUnstructured(serviceExport)
	panicOnError(err)

	_, err = serviceExportClient.Namespace(seNamespace).UpdateStatus(context.TODO(), raw, metav1.UpdateOptions{})
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
