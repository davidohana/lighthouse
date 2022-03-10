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
)

var _ = FDescribe("ServiceImport syncing", func() {
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
		It("should download the ServiceImport to all clusters, create EndpointSlice in origin cluster and sync to broker and other cluster", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createEndpoints()
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

			t.createEndpoints()
			t.awaitEndpointSlice()
		})
	})

	When("a ServiceImport is deleted on the broker", func() {
		It("should delete the local import and EndpointSlice and sync to broker and other cluster", func() {
			t.awaitNoEndpointSlice()
			t.awaitNoServiceImport()

			t.createEndpoints()
			t.createBrokerServiceImport()
			t.awaitServiceImport()
			t.awaitEndpointSlice()

			t.deleteBrokerServiceImport()
			t.awaitNoServiceImport()
			t.awaitNoEndpointSlice()
		})
	})

	FWhen("broker service import is deleted out of band after sync", func() {
		It("should delete it from the clusters datastore on reconciliation", func() {
			// simulate sync of import from broker to client to get expected local service import state
			t.createBrokerServiceImport()
			localServiceImport1 := t.awaitServiceImportOnClient(t.cluster1.serviceImportClient)
			localServiceImport2 := t.awaitServiceImportOnClient(t.cluster1.serviceImportClient)

			t.afterEach()                                                            // stop agent controller on all clusters
			t = newTestDriver()                                                      // create a new driver - data stores are now empty
			test.CreateResource(t.cluster1.serviceImportClient, localServiceImport1) // create headless import on cluster1
			test.CreateResource(t.cluster2.serviceImportClient, localServiceImport2) // create headless import on cluster2
			t.justBeforeEach()                                                       // start agent controller on all clusters
			t.awaitNoServiceImport()                                                 // assert that imports are deleted
		})
	})

})
