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
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/syncer/test"
)

var _ = Describe("Cleanup", func() {
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

	It("should delete synced objects as expected", func() {
		serviceNameCluster1 := t.getServiceNameCluster1()
		serviceNameCluster2 := t.getServiceNameCluster2()

		logger.Info("Creating services, endpoints, exports, imports")
		test.CreateResource(t.cluster1.dynamicServiceClient().Namespace(t.service.Namespace), t.service)
		test.CreateResource(t.cluster2.dynamicServiceClient().Namespace(t.service.Namespace), t.service)
		test.CreateResource(t.cluster1.serviceExportClient, t.serviceExport)
		test.CreateResource(t.cluster2.serviceExportClient, t.serviceExport)
		test.CreateResource(t.dynamicEndpointsClient(&t.cluster1), t.endpoints)
		test.CreateResource(t.dynamicEndpointsClient(&t.cluster2), t.endpoints)
		test.CreateResource(t.brokerServiceImportClient, t.serviceImportCluster1)
		test.CreateResource(t.brokerServiceImportClient, t.serviceImportCluster2)

		logger.Info("Checking that all objects are synced")
		t.awaitServiceExport(t.cluster1.serviceExportClient, t.service.Name, nil)
		t.awaitServiceExport(t.cluster2.serviceExportClient, t.service.Name, nil)
		t.awaitServiceExport(t.brokerServiceExportClient, serviceNameCluster1, nil)
		t.awaitServiceExport(t.brokerServiceExportClient, serviceNameCluster2, nil)

		test.AwaitResource(t.brokerServiceImportClient, serviceNameCluster1)
		test.AwaitResource(t.brokerServiceImportClient, serviceNameCluster2)
		test.AwaitResource(t.cluster1.serviceImportClient, serviceNameCluster1)
		test.AwaitResource(t.cluster1.serviceImportClient, serviceNameCluster2)
		test.AwaitResource(t.cluster2.serviceImportClient, serviceNameCluster1)
		test.AwaitResource(t.cluster2.serviceImportClient, serviceNameCluster2)

		t.awaitBrokerEndpointSlice(clusterID1)
		t.awaitBrokerEndpointSlice(clusterID2)
		t.awaitLocalEndpointSlice(t.cluster1.endpointSliceClient, clusterID1)
		t.awaitLocalEndpointSlice(t.cluster1.endpointSliceClient, clusterID2)
		t.awaitLocalEndpointSlice(t.cluster2.endpointSliceClient, clusterID1)
		t.awaitLocalEndpointSlice(t.cluster2.endpointSliceClient, clusterID2)

		logger.Info("Executing cleanup on cluster1")
		Expect(t.cluster1.agentController.Cleanup()).To(Succeed())

		logger.Info("Asserting local exports are not deleted")
		t.awaitServiceExport(t.cluster1.serviceExportClient, t.service.Name, nil)
		t.awaitServiceExport(t.cluster2.serviceExportClient, t.service.Name, nil)

		logger.Info("Asserting broker copy of service export is deleted only for cleaned cluster")
		test.AwaitNoResource(t.brokerServiceExportClient, serviceNameCluster1)
		t.awaitServiceExport(t.brokerServiceExportClient, serviceNameCluster2, nil)

		logger.Info("Asserting local imports are deleted on cluster1 only")
		test.AwaitNoResource(t.cluster1.serviceImportClient, serviceNameCluster1)
		test.AwaitNoResource(t.cluster1.serviceImportClient, serviceNameCluster2)
		test.AwaitResource(t.cluster2.serviceImportClient, serviceNameCluster1)
		test.AwaitResource(t.cluster2.serviceImportClient, serviceNameCluster2)
		test.AwaitResource(t.brokerServiceImportClient, serviceNameCluster1)
		test.AwaitResource(t.brokerServiceImportClient, serviceNameCluster2)

		logger.Info("Asserting all ep slices are deleted from cluster1")
		epsNameCluster1 := t.service.Name + "-" + clusterID1
		epsNameCluster2 := t.service.Name + "-" + clusterID2
		test.AwaitNoResource(t.cluster1.endpointSliceClient, epsNameCluster1)
		test.AwaitNoResource(t.cluster1.endpointSliceClient, epsNameCluster2)

		logger.Info("Asserting ep slices originated from cluster1 are deleted on all clusters")
		test.AwaitNoResource(t.brokerEndpointSliceClient, epsNameCluster1)
		test.AwaitNoResource(t.cluster2.endpointSliceClient, epsNameCluster1)

		logger.Info("Asserting ep slices originated from cluster2 still exist on broker and cluster1")
		t.awaitBrokerEndpointSlice(clusterID2)
		t.awaitLocalEndpointSlice(t.cluster2.endpointSliceClient, clusterID2)
	})
})
