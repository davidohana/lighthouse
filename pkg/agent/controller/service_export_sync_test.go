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
	"github.com/submariner-io/lighthouse/pkg/agent/controller"
	"github.com/submariner-io/lighthouse/pkg/mcs"
	corev1 "k8s.io/api/core/v1"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	"time"
)
import . "github.com/onsi/gomega"

var _ = Describe("ServiceExport syncing", func() {
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

	When("a ServiceExport is created", func() {
		When("the Service already exists", func() {
			It("should update the ServiceExport status and upload to the broker", func() {
				t.awaitNoServiceExportOnBroker()
				t.createService()
				t.createServiceExport()
				t.awaitServiceExported()
			})
		})

		When("the Service doesn't initially exist", func() {
			It("should initially update the ServiceExport status to Valid and upload after service is created", func() {
				t.createServiceExport()
				t.awaitServiceUnavailableStatus()
				t.awaitNoServiceExportOnBroker()

				t.createService()
				t.awaitServiceExported()
			})
		})
	})

	When("a ServiceExport is deleted after it was synced", func() {
		It("should delete the ServiceExport on the Broker", func() {
			t.createService()
			t.createServiceExport()
			t.awaitServiceExported()

			t.deleteServiceExport()
			t.awaitNoServiceExportOnBroker()
		})
	})

	When("an exported Service is deleted and recreated while the ServiceExport still exists", func() {
		It("should delete and recreate the ServiceExport on the broker", func() {
			t.createService()
			t.createServiceExport()
			t.awaitServiceExported()

			t.deleteService()
			t.awaitNoServiceExportOnBroker()
			t.awaitServiceUnavailableStatus()

			t.createService()
			t.awaitServiceExported()
		})
	})

	When("the ServiceExport sync initially fails", func() {
		BeforeEach(func() {
			t.brokerServiceExportClient.PersistentFailOnCreate.Store("mock create error")
		})

		It("should not update the ServiceExport status to Exported until the sync is successful", func() {
			t.createService()
			t.createServiceExport()

			condition := newServiceExportCondition(mcsv1a1.ServiceExportValid, corev1.ConditionFalse, controller.ReasonAwaitingSync)
			t.awaitLocalServiceExport(condition)

			t.awaitNoServiceExportOnBroker()

			t.brokerServiceExportClient.PersistentFailOnCreate.Store("")
			t.awaitServiceExported()
		})
	})

	When("the ServiceExportCondition list count reaches MaxExportStatusConditions", func() {
		var oldMaxExportStatusConditions int

		BeforeEach(func() {
			oldMaxExportStatusConditions = controller.MaxExportStatusConditions
			controller.MaxExportStatusConditions = 1
		})

		AfterEach(func() {
			controller.MaxExportStatusConditions = oldMaxExportStatusConditions
		})

		It("should correctly truncate the ServiceExportCondition list", func() {
			t.createService()
			t.createServiceExport()

			t.awaitServiceExported()

			serviceExport := t.awaitLocalServiceExport(nil)
			Expect(len(serviceExport.Status.Conditions)).To(Equal(1))
		})
	})

	When("a ServiceExport is created for a Service whose type is other than ServiceTypeClusterIP", func() {
		BeforeEach(func() {
			t.service.Spec.Type = corev1.ServiceTypeNodePort
		})

		It("should update the ServiceExport status and not sync it", func() {
			t.createService()
			t.createServiceExport()

			condition := newServiceExportCondition(mcsv1a1.ServiceExportValid, corev1.ConditionFalse, controller.ReasonUnsupportedServiceType)
			t.awaitLocalServiceExport(condition)

			t.awaitNoServiceExportOnBroker()
		})
	})

	When("a Service has port information", func() {
		BeforeEach(func() {
			t.service.Spec.Ports = []corev1.ServicePort{
				{
					Name:     "eth0",
					Protocol: corev1.ProtocolTCP,
					Port:     123,
				},
				{
					Name:     "eth1",
					Protocol: corev1.ProtocolSCTP,
					Port:     1234,
				},
			}
		})

		It("should set the appropriate port information in the ServiceExport", func() {
			t.createService()
			t.createServiceExport()
			t.awaitServiceExported() // t.service.Spec.ClusterIP, 0

			serviceExport := t.awaitBrokerServiceExport(nil)
			exportSpec := &mcs.ExportSpec{}
			err := exportSpec.UnmarshalObjectMeta(&serviceExport.ObjectMeta)
			Expect(err).To(BeNil())
			Expect(len(exportSpec.Service.Ports)).To(Equal(2))
			Expect(exportSpec.Service.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(exportSpec.Service.Ports[1].Protocol).To(Equal(corev1.ProtocolSCTP))
			Expect(exportSpec.Service.Ports[0].Port).To(BeNumerically("==", 123))
			Expect(exportSpec.Service.Ports[1].Port).To(BeNumerically("==", 1234))
			Expect(time.Now().Sub(exportSpec.CreatedAt.Time)).To(BeNumerically("<", 60*time.Second))
		})
	})
})
