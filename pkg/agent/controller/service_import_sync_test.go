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
		It("should download the ServiceImport, create EndpointSlice and sync to broker and other cluster", func() {
			t.createEndpoints()
			t.awaitNoEndpointSlice(t.cluster1.localEndpointSliceClient)
			t.awaitNoServiceImport(t.brokerServiceImportClient)
			t.createBrokerServiceImport()
			t.awaitLocalServiceImport()
			t.awaitEndpointSlice()
		})
	})

	When("a ServiceImport is created on broker when local Endpoints does not exist", func() {
		It("should download the ServiceImport", func() {
			t.awaitNoEndpointSlice(t.cluster1.localEndpointSliceClient)
			t.awaitNoServiceImport(t.brokerServiceImportClient)
			t.createBrokerServiceImport()
			t.awaitLocalServiceImport()
			t.awaitNoEndpointSlice(t.cluster1.localEndpointSliceClient)
		})
	})

	When("local endpoints created when local import already exist", func() {
		It("should create EndpointSlice and sync to broker and other cluster", func() {
			t.awaitNoEndpointSlice(t.cluster1.localEndpointSliceClient)
			t.awaitNoServiceImport(t.brokerServiceImportClient)
			t.createBrokerServiceImport()
			t.awaitLocalServiceImport()
			t.awaitNoEndpointSlice(t.cluster1.localEndpointSliceClient)
			t.createEndpoints()
			t.awaitEndpointSlice()
		})
	})

})
