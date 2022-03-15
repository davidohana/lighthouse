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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	mcsclient "sigs.k8s.io/mcs-api/pkg/client/clientset/versioned"
)

func main() {
	condType := string(mcsv1a1.ServiceExportConflict)
	condReason := "default reason"
	condMsg := "default message"
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

	clientset := kubernetes.NewForConfigOrDie(restConfig)

	// list pods for sanity check, to verify that we can reach the cluster
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	panicOnError(err)
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	err = mcsv1a1.AddToScheme(scheme.Scheme)
	panicOnError(err)

	mcsClient := mcsclient.NewForConfigOrDie(restConfig)
	nsExports := mcsClient.MulticlusterV1alpha1().ServiceExports(seNamespace)
	se, err := nsExports.Get(context.TODO(), seName, metav1.GetOptions{})
	panicOnError(err)

	fmt.Printf("Found %d conditions in ServiceExport\n", len(se.Status.Conditions))

	now := metav1.Now()
	exportCondition := mcsv1a1.ServiceExportCondition{
		LastTransitionTime: &now,
		Type:               mcsv1a1.ServiceExportConditionType(condType),
		Status:             corev1.ConditionStatus(condStatus),
		Reason:             &condReason,
		Message:            &condMsg,
	}

	se.Status.Conditions = append(se.Status.Conditions, exportCondition)
	fmt.Printf("now: %d conditions in ServiceExport\n", len(se.Status.Conditions))

	_, err = nsExports.UpdateStatus(context.TODO(), se, metav1.UpdateOptions{})
	panicOnError(err)

	println("Done")
}

func panicOnError(err error) {
	if err != nil {
		panic(err.Error())
	}
}
