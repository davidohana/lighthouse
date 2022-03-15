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
package controller_test

import (
	. "github.com/onsi/ginkgo"
	"github.com/submariner-io/admiral/pkg/syncer/test"
	corev1 "k8s.io/api/core/v1"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var _ = Describe("ServiceImport syncing", func() {
	var t *testDriver

	BeforeEach(func() {
		t = newTestDriver()
	})

	JustBeforeEach(func() {
		t.justBeforeEach()
	})

	AfterEach(func() {
		t.afterEach()
	})

	When("a ServiceImport is created on broker when local Endpoints exist", func() {
		It("should download the ServiceImport to all clusters, create EndpointSlice in origin cluster "+
			"and sync to broker and other cluster", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createEndpointsOnCluster1()
			t.createBrokerServiceImport()
			t.awaitServiceImport()
			t.awaitEndpointSlice()
		})
	})

	When("a headless ServiceImport is created on broker when local Endpoints exist", func() {
		JustBeforeEach(func() {
			t.serviceImportCluster1.Spec.Type = mcsv1a1.Headless
		})

		JustAfterEach(func() {
			t.serviceImportCluster1.Spec.Type = mcsv1a1.ClusterSetIP
		})

		It("should download the ServiceImport to all clusters, create EndpointSlice in origin cluster "+
			"and sync to broker and other cluster", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createEndpointsOnCluster1()
			t.createBrokerServiceImport()
			t.awaitServiceImport()
			t.awaitEndpointSlice()
		})
	})

	When("a ServiceImport is created on broker when local Endpoints does not exist", func() {
		It("should download the ServiceImport to all clusters", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createBrokerServiceImport()
			t.awaitServiceImport()
			t.awaitNoEndpointSlice()
		})
	})

	When("local endpoints created when local import already exist", func() {
		It("should create EndpointSlice and sync to broker and other cluster", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createBrokerServiceImport()
			t.awaitServiceImport()
			t.awaitNoEndpointSlice()

			t.createEndpointsOnCluster1()
			t.awaitEndpointSlice()
		})
	})

	When("local endpoints deleted while an EndpointSlice exist", func() {
		It("should delete endpoint slice from all clusters", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createEndpointsOnCluster1()
			t.createBrokerServiceImport()
			t.awaitServiceImport()
			t.awaitEndpointSlice()

			t.deleteEndpointsOnCluster1()
			t.awaitNoEndpointSlice()
		})
	})

	When("local endpoints updated while an EndpointSlice exist", func() {
		It("should update endpoint slice in all clusters", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createEndpointsOnCluster1()
			t.createBrokerServiceImport()
			t.awaitServiceImport()
			t.awaitEndpointSlice()

			t.endpoints.Subsets[0].Addresses[0].IP = "1.2.3.4"
			t.updateEndpoints()
			t.awaitUpdatedEndpointSlice()

			t.endpoints.Subsets[0].NotReadyAddresses = append(t.endpoints.Subsets[0].NotReadyAddresses, corev1.EndpointAddress{IP: "7.7.7.7"})
			t.updateEndpoints()
			t.awaitUpdatedEndpointSlice()
		})
	})

	When("a ServiceImport is deleted on the broker", func() {
		It("should delete the local import and EndpointSlice and sync to broker and other cluster", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createEndpointsOnCluster1()
			t.createBrokerServiceImport()
			t.awaitServiceImport()
			t.awaitEndpointSlice()

			t.deleteBrokerServiceImport()
			t.awaitNoServiceImport()
			t.awaitNoEndpointSlice()
		})
	})

	When("broker service import is deleted out of band after sync", func() {
		It("should delete import and ep slices from the clusters datastore on reconciliation", func() {
			// simulate sync of import from broker to client to get expected local state for import and ep slice
			t.createBrokerServiceImport()
			t.createEndpointsOnCluster1()
			localServiceImport1 := t.awaitServiceImportOnClient(t.cluster1.serviceImportClient)
			localServiceImport2 := t.awaitServiceImportOnClient(t.cluster1.serviceImportClient)
			localEpSlice1 := t.awaitLocalEndpointSlice(t.cluster1.endpointSliceClient, clusterID1)
			localEpSlice2 := t.awaitLocalEndpointSlice(t.cluster2.endpointSliceClient, clusterID1)

			t.afterEach()                                                            // stop agent controller on all clusters
			t = newTestDriver()                                                      // create a new driver - data stores are now empty
			test.CreateResource(t.cluster1.serviceImportClient, localServiceImport1) // create headless import on cluster1
			test.CreateResource(t.cluster2.serviceImportClient, localServiceImport2) // create headless import on cluster2
			test.CreateResource(t.cluster1.endpointSliceClient, localEpSlice1)       // create endpoint slice on cluster1
			test.CreateResource(t.cluster2.endpointSliceClient, localEpSlice2)       // create endpoint slice on cluster2
			t.justBeforeEach()                                                       // start agent controller on all clusters
			t.awaitNoServiceImport()                                                 // assert that imports are deleted
			t.awaitNoEndpointSlice()                                                 // assert that ep slices are deleted
		})
	})

	When("broker service import is created out of band", func() {
		It("should sync it to the clusters datastore on reconciliation", func() {
			t.afterEach()       // stop agent controller on all clusters
			t = newTestDriver() // create a new driver - data stores are now empty

			logger.Info("Creating import on broker oob")
			t.createBrokerServiceImport()

			logger.Info("create endpoints on origin cluster")
			t.createEndpointsOnCluster1()

			logger.Info("start agent controller on all clusters")
			t.justBeforeEach()

			logger.Info("Asserting")
			t.awaitServiceImport() // assert that import is synced to clusters
			t.awaitEndpointSlice() // assert that ep slice is created and synced to other clusters
		})
	})

	When("local owned ep slice is deleted out of band", func() {
		It("should delete ep slice from broker and other cluster as well", func() {
			t.createBrokerServiceImport()
			t.createEndpointsOnCluster1()
			t.awaitServiceImportOnClient(t.cluster1.serviceImportClient)
			brokerEpSlice := t.awaitBrokerEndpointSlice(clusterID1)
			localEpSlice2 := t.awaitLocalEndpointSlice(t.cluster2.endpointSliceClient, clusterID1)

			t.afterEach()                                                      // stop agent controller on all clusters
			t = newTestDriver()                                                // create a new driver - data stores are now empty
			test.CreateResource(t.brokerEndpointSliceClient, brokerEpSlice)    // create endpoint slice on broker oob
			test.CreateResource(t.cluster2.endpointSliceClient, localEpSlice2) // create endpoint slice on other cluster oob
			t.justBeforeEach()                                                 // start agent controller on all clusters
			t.awaitNoEndpointSlice()                                           // ensure sp slice is deleted from all places
		})
	})

	When("broker ep slice is deleted out of band", func() {
		It("should upload it again", func() {
			t.createBrokerServiceImport()
			t.createEndpointsOnCluster1()
			t.awaitServiceImportOnClient(t.cluster1.serviceImportClient)
			localEpSlice1 := t.awaitLocalEndpointSlice(t.cluster1.endpointSliceClient, clusterID1)

			t.afterEach()                                                      // stop agent controller on all clusters
			t = newTestDriver()                                                // create a new driver - data stores are now empty
			test.CreateResource(t.cluster1.endpointSliceClient, localEpSlice1) // create endpoint slice on the origin cluster oob
			t.justBeforeEach()                                                 // start agent controller on all clusters
			t.awaitEndpointSlice()                                             // ensure sp slice is synced to broker and other cluster
		})
	})

	When("broker ep slice is deleted oob + import deleted from local clusters", func() {
		It("should sync ep slice and import on reconciliation", func() {
			t.createBrokerServiceImport()
			t.createEndpointsOnCluster1()
			t.awaitServiceImportOnClient(t.cluster1.serviceImportClient)
			localEpSlice1 := t.awaitLocalEndpointSlice(t.cluster1.endpointSliceClient, clusterID1)

			t.afterEach()                                                      // stop agent controller on all clusters
			t = newTestDriver()                                                // create a new driver - data stores are now empty
			t.createBrokerServiceImport()                                      // create import on broker oob
			test.CreateResource(t.cluster1.endpointSliceClient, localEpSlice1) // create endpoint slice on cluster1 oob
			t.justBeforeEach()                                                 // start agent controller on all clusters
			t.awaitEndpointSlice()                                             // ensure sp slice is synced to other clusters
			t.awaitServiceImport()                                             // ensure import is synced to clusters
		})
	})
})
